/*
 * bitfinex_private.go - Bitfinex Private client
 *
 * bitfinex_borrow_catcher - Automatic borrow catcher for open positions in
 *                            the Bitfinex exchange
 * Copyright (C) 2021  Mateusz Szpakowski
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301  USA
 */

package main

import (
    "crypto/hmac"
    "crypto/sha512"
    "encoding/hex"
    "strconv"
    "time"
    "github.com/matszpk/godec64"
    "github.com/valyala/fasthttp"
    "github.com/valyala/fastjson"
)

var (
    bitfinexPrivApiHost = []byte("api.bitfinex.com")
    bitfinexStrNonce = []byte("bfx-nonce")
    bitfinexStrApiKey = []byte("bfx-apikey")
    bitfinexStrSignature = []byte("bfx-signature")
    bitfinexStrApiPrefix = []byte("/api/")
    bitfinexStrEmptyJson = []byte("{}")
    bitfinexApiWallets = []byte("v2/auth/r/wallets")
    bitfinexApiFundingLoans = []byte("v2/auth/r/funding/loans/f")
    bitfinexApiFundingCredits = []byte("v2/auth/r/funding/credits/f")
    bitfinexApiFundingTrades = []byte("v2/auth/r/funding/trades/f")
    bitfinexApiPositions = []byte("v2/auth/r/positions")
    bitfinexApiFundingClose = []byte("v2/auth/w/funding/close")
    bitfinexApiSubmit = []byte("v2/auth/w/funding/offer/submit")
    bitfinexApiCancel = []byte("v2/auth/w/funding/offer/cancel")
    bitfinexApiOrders = []byte("v2/auth/r/funding/offers/f")
    bitfinexStrSUCCESS = []byte("SUCCESS")
)

type Balance struct {
    Currency string
    Type string
    Total godec64.UDec64
    Available godec64.UDec64
}

type Loan struct {
    Id uint64
    Currency string
    Side int
    CreateTime time.Time
    UpdateTime time.Time
    Amount godec64.UDec64
    Status string
    Rate godec64.UDec64
    Period uint32
    Renew bool
    NoClose bool
}

type Credit struct {
    Loan
    Market string
}

type OrderStatus uint8

const (
    OrderActive = iota
    OrderExecuted
    OrderPartiallyFilled
    OrderCanceled
)

type Order struct {
    Id uint64
    Currency string
    CreateTime time.Time
    UpdateTime time.Time
    Amount godec64.UDec64
    AmountOrig godec64.UDec64
    Status OrderStatus
    Rate godec64.UDec64
    Period uint32
    Renew bool
}

type OpResult struct {
    Order Order
    Success bool
    Message string
}

type Op2Result struct {
    Success bool
    Message string
}

type Position struct {
    Id uint64
    Market string
    Status string
    Amount godec64.UDec64
    Long bool
    BasePrice godec64.UDec64
    Funding godec64.UDec64
    LiqPrice godec64.UDec64
}

type BitfinexPrivate struct {
    httpClient fasthttp.HostClient
    apiKey, apiSecret []byte
}

func NewBitfinexPrivate(apiKey, apiSecret []byte) *BitfinexPrivate {
    return &BitfinexPrivate{ httpClient: fasthttp.HostClient{
        Addr: "api.bitfinex.com,api-pub.bitfinex.com",
        IsTLS: true, ReadTimeout: time.Second*60 },
        apiKey: apiKey, apiSecret: apiSecret }
}

func (drv *BitfinexPrivate) handleHttpPostJson(rh *RequestHandle,
                host, uri, query []byte, bodyStr []byte) (*fastjson.Value, int) {
    nonceB := strconv.AppendInt(nil ,time.Now().UnixNano()/100000, 10)
    // generate signature
    sig := make([]byte, 0, 200)
    sig = append(sig, bitfinexStrApiPrefix...)
    sig = append(sig, uri...)
    sig = append(sig, nonceB...)
    sig = append(sig, bodyStr...)
    
    sumGen := hmac.New(sha512.New384, drv.apiSecret)
    if _, err := sumGen.Write(sig); err!=nil {
        ErrorPanic("Error while generating signature hash:", err)
    }
    sum := sumGen.Sum(nil)
    sumHex := make([]byte, len(sum)*2)
    hex.Encode(sumHex, sum)
    
    headers := [][]byte{
        bitfinexStrNonce, nonceB,
        bitfinexStrApiKey, drv.apiKey,
        bitfinexStrSignature, sumHex }
    
    return rh.HandleHttpPostJson(&drv.httpClient, host, uri, query, bodyStr, headers)
}

func bitfinexGetBalanceFromJson(v *fastjson.Value, bal *Balance) {
    arr := FastjsonGetArray(v)
    if len(arr) < 7 {
        panic("Wrong json body")
    }
    *bal = Balance{}
    
    bal.Currency = FastjsonGetString(arr[1])
    bal.Type = FastjsonGetString(arr[0])
    t, m := FastjsonGetUDec64Signed(arr[2], 8)
    if !m { bal.Total = t }
    bal.Available = FastjsonGetUDec64(arr[4], 8)
}

func (drv *BitfinexPrivate) GetMarginBalances() []Balance {
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, bitfinexApiWallets, nil,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get margin balances", v, sc) }
    
    arr := FastjsonGetArray(v)
    bals := make([]Balance, 0)
    
    for _, v := range arr {
        var bal Balance
        bitfinexGetBalanceFromJson(v, &bal)
        if bal.Type == "margin" {
            bals = append(bals, bal)
        }
    }
    return bals
}

func bitfinexGetLoanFromJson(v *fastjson.Value, loan *Loan) {
    arr := FastjsonGetArray(v)
    if len(arr) < 21 {
        panic("Wrong json body")
    }
    *loan = Loan{}
    loan.Id = FastjsonGetUInt64(arr[0])
    loan.Currency = FastjsonGetString(arr[1])[1:]
    loan.Side = FastjsonGetInt(arr[2])
    loan.CreateTime = FastjsonGetUnixTimeMilli(arr[3])
    loan.UpdateTime = FastjsonGetUnixTimeMilli(arr[4])
    loan.Amount = FastjsonGetUDec64(arr[5], 8)
    loan.Status = FastjsonGetString(arr[7])
    loan.Rate = FastjsonGetUDec64(arr[11], 12)
    loan.Period = FastjsonGetUInt32(arr[12])
    loan.Renew = FastjsonGetUInt32(arr[18])!=0
    loan.NoClose = FastjsonGetUInt32(arr[20])!=0
}

func (drv *BitfinexPrivate) GetLoans(currency string) []Loan {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingLoans...)
    apiUrl = append(apiUrl, currency...)
        
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, nil,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get funding loans", v, sc) }
    
    arr := FastjsonGetArray(v)
    loansLen := len(arr)
    loans := make([]Loan, loansLen)
    
    for i, v := range arr {
        bitfinexGetLoanFromJson(v, &loans[i])
    }
    return loans
}

func (drv *BitfinexPrivate) GetLoansHistory(currency string,
                                since time.Time, limit uint) []Loan {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingLoans...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/hist"...)
    body := make([]byte, 0, 40)
    body = append(body, `{"limit":`...)
    body = strconv.AppendUint(body, uint64(limit), 10)
    if !since.IsZero() {
        unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
        body = append(body, `,"start":`...)
        body = strconv.AppendInt(body, unixTime, 10)
    }
    body = append(body, '}')
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, nil, body)
    if sc >= 400 { bitfinexPanic("Can't get funding loans history", v, sc) }
    
    arr := FastjsonGetArray(v)
    loansLen := len(arr)
    loans := make([]Loan, loansLen)
    
    for i, v := range arr {
        bitfinexGetLoanFromJson(v, &loans[loansLen-i-1])
    }
    return loans
}

func bitfinexGetCreditFromJson(v *fastjson.Value, credit *Credit) {
    arr := FastjsonGetArray(v)
    if len(arr) < 22 {
        panic("Wrong json body")
    }
    *credit = Credit{}
    credit.Id = FastjsonGetUInt64(arr[0])
    credit.Currency = FastjsonGetString(arr[1])[1:]
    credit.Side = FastjsonGetInt(arr[2])
    credit.CreateTime = FastjsonGetUnixTimeMilli(arr[3])
    credit.UpdateTime = FastjsonGetUnixTimeMilli(arr[4])
    credit.Amount = FastjsonGetUDec64(arr[5], 8)
    credit.Status = FastjsonGetString(arr[7])
    credit.Rate = FastjsonGetUDec64(arr[11], 12)
    credit.Period = FastjsonGetUInt32(arr[12])
    credit.Renew = FastjsonGetUInt32(arr[18])!=0
    credit.NoClose = FastjsonGetUInt32(arr[20])!=0
    credit.Market = FastjsonGetString(arr[21])[1:]
}

func (drv *BitfinexPrivate) GetCredits(currency string) []Credit {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingCredits...)
    apiUrl = append(apiUrl, currency...)
        
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, nil,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get funding credits", v, sc) }
    
    arr := FastjsonGetArray(v)
    creditsLen := len(arr)
    credits := make([]Credit, creditsLen)
    
    for i, v := range arr {
        bitfinexGetCreditFromJson(v, &credits[i])
    }
    return credits
}

func (drv *BitfinexPrivate) GetCreditsHistory(currency string,
                                since time.Time, limit uint) []Credit {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingCredits...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/hist"...)
    body := make([]byte, 0, 40)
    body = append(body, `{"limit":`...)
    body = strconv.AppendUint(body, uint64(limit), 10)
    if !since.IsZero() {
        unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
        body = append(body, `,"start":`...)
        body = strconv.AppendInt(body, unixTime, 10)
    }
    body = append(body, '}')
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, nil, body)
    if sc >= 400 { bitfinexPanic("Can't get funding credits history", v, sc) }
    
    arr := FastjsonGetArray(v)
    creditsLen := len(arr)
    credits := make([]Credit, creditsLen)
    
    for i, v := range arr {
        bitfinexGetCreditFromJson(v, &credits[creditsLen-i-1])
    }
    return credits
}

func bitfinexGetOrderFromJson(v *fastjson.Value, order *Order) {
    arr := FastjsonGetArray(v)
    if len(arr) < 20 {
        panic("Wrong json body")
    }
    *order = Order{}
    order.Id = FastjsonGetUInt64(arr[0])
    order.Currency = FastjsonGetString(arr[1])[1:]
    order.CreateTime = FastjsonGetUnixTimeMilli(arr[2])
    order.UpdateTime = FastjsonGetUnixTimeMilli(arr[3])
    order.Amount, _ = FastjsonGetUDec64Signed(arr[4], 8)
    order.AmountOrig, _ = FastjsonGetUDec64Signed(arr[5], 8)
    status := FastjsonGetString(arr[10])
    switch status {
        case "ACTIVE":
            order.Status = OrderActive
        case "EXECUTED":
            order.Status = OrderExecuted
        case "PARTIALLY FILLED":
            order.Status = OrderPartiallyFilled
        case "CANCELED":
            order.Status = OrderCanceled
        default:
            panic("Unknown order status")
    }
    order.Rate = FastjsonGetUDec64(arr[14], 12)
    order.Period = FastjsonGetUInt32(arr[15])
    if arr[19].Type() == fastjson.TypeNumber {
        order.Renew = FastjsonGetInt(arr[19])!=0
    } else {
        order.Renew = FastjsonGetBool(arr[19])
    }
}

func (drv *BitfinexPrivate) CloseFunding(loanId uint64, or *Op2Result) {
    body := make([]byte, 0, 30)
    body = append(body, `{"id":`...)
    body = strconv.AppendUint(body, loanId, 10)
    body = append(body, '}')
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost,
                                    bitfinexApiFundingClose, nil, body)
    if sc >= 400 { bitfinexPanic("Can't close funding", v, sc) }
    
    // parse submit result
    arr := FastjsonGetArray(v)
    if len(arr) < 8 {
        panic("Wrong json body")
    }
    
    *or = Op2Result{}
    or.Success = FastjsonCheckString(arr[6], bitfinexStrSUCCESS)
}

func (drv *BitfinexPrivate) SubmitBidOrder(currency string,
                            amount,rate godec64.UDec64, period uint32,
                            or *OpResult) {
    body := make([]byte, 0, 80)
    body = append(body, `{"type":"LIMIT","symbol":"f`...)
    body = append(body, currency...)
    body = append(body, `","amount":"-`...)
    body = append(body, amount.FormatBytes(8, false)...)
    body = append(body, `","rate":"`...)
    body = append(body, rate.FormatBytes(12, false)...)
    body = append(body, `","period":`...)
    body = strconv.AppendUint(body, uint64(period), 10)
    body = append(body, `,"flags":0}`...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost,
                                    bitfinexApiSubmit, nil, body)
    if sc >= 400 { bitfinexPanic("Can't submit order", v, sc) }
    
    // parse submit result
    arr := FastjsonGetArray(v)
    if len(arr) < 8 {
        panic("Wrong json body")
    }
    
    *or = OpResult{}
    bitfinexGetOrderFromJson(arr[4], &or.Order)
    or.Success = FastjsonCheckString(arr[6], bitfinexStrSUCCESS)
    or.Message = FastjsonGetString(arr[7])
}

func (drv *BitfinexPrivate) CancelOrder(orderId uint64, or *OpResult) {
    body := make([]byte, 0, 30)
    body = append(body, `{"id":`...)
    body = strconv.AppendUint(body, orderId, 10)
    body = append(body, '}')
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost,
                                    bitfinexApiCancel, nil, body)
    if sc >= 400 { bitfinexPanic("Can't cancel order", v, sc) }
    
    // parse submit result
    arr := FastjsonGetArray(v)
    if len(arr) < 8 {
        panic("Wrong json body")
    }
    
    *or = OpResult{}
    bitfinexGetOrderFromJson(arr[4], &or.Order)
    or.Success = FastjsonCheckString(arr[6], bitfinexStrSUCCESS)
    or.Message = FastjsonGetString(arr[7])
}

func (drv *BitfinexPrivate) GetActiveOrders(currency string) []Order {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiOrders...)
    apiUrl = append(apiUrl, currency...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, nil,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get orders", v, sc) }
    
    arr := FastjsonGetArray(v)
    ordersLen := len(arr)
    orders := make([]Order, ordersLen)
    for i, v := range arr {
        bitfinexGetOrderFromJson(v, &orders[i])
    }
    return orders
}

func bitfinexGetPositionFromJson(v *fastjson.Value, pos *Position) {
    arr := FastjsonGetArray(v)
    if len(arr) < 19 {
        panic("Wrong json body")
    }
    *pos = Position{}
    pos.Id = FastjsonGetUInt64(arr[11])
    pos.Market = FastjsonGetString(arr[0])[1:]
    amount, neg := FastjsonGetUDec64Signed(arr[2], 8)
    pos.Long = !neg
    pos.Amount = amount
    pos.BasePrice, neg = FastjsonGetUDec64Signed(arr[3], 8)
    if neg { pos.BasePrice = 0 }
    pos.Funding, _ = FastjsonGetUDec64Signed(arr[4], 8)
    pos.LiqPrice = FastjsonGetUDec64(arr[8], 8)
    pos.Status = FastjsonGetString(arr[1])
}

func (drv *BitfinexPrivate) GetPositions() []Position {
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, bitfinexApiPositions,
                                    nil, bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get positions", v, sc) }
    
    arr := FastjsonGetArray(v)
    posLen := len(arr)
    poss := make([]Position, posLen)
    for i, v := range arr {
        bitfinexGetPositionFromJson(v, &poss[i])
    }
    return poss
}
