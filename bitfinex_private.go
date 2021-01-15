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
    "fmt"
    "strconv"
    "time"
    "github.com/matszpk/godec128"
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
    bitfinexApiFundingLoans = []byte("v2/auth/r/funding/loans/f")
    bitfinexApiFundingCredits = []byte("v2/auth/r/funding/credits/f")
    bitfinexApiFundingTrades = []byte("v2/auth/r/funding/trades/f")
    bitfinexApiSubmit = []byte("v2/auth/w/funding/offer/submit")
)

type Loan struct {
    Id uint64
    Currency string
    Side int
    CreateTime time.Time
    UpdateTime time.Time
    Amount godec128.UDec128
    Status string
    Rate godec128.UDec128
    Period uint32
    Renew bool
    NoClose bool
}

type Credit struct {
    Loan
    Market string
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
    
    return rh.HandleHttpPostJson(drv.httpClient, host, uri, query, bodyStr, headers)
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
    loan.Amount = FastjsonGetUDec128(arr[5], 8)
    loan.Status = FastjsonGetString(arr[7])
    loan.Rate = FastjsonGetUDec128(arr[11], 8)
    loan.Period = FastjsonGetUInt32(arr[12])
    loan.Renew = FastjsonGetUInt32(arr[18])!=0
    loan.NoClose = FastjsonGetUInt32(arr[20])!=0
}

func (drv *BitfinexPrivate) GetFundingLoans(currency string) []Loan {
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

func (drv *BitfinexPrivate) GetFundingLoansHistory(currency string,
                                since time.Time, limit uint) []Loan {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingLoans...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/hist"...)
    query := make([]byte, 0, 40)
    query = append(query, "?limit="...)
    query = strconv.AppendUint(query, uint64(limit), 10)
    if !since.IsZero() {
        unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
        query = append(query, "&start="...)
        query = strconv.AppendInt(query, unixTime, 10)
    }
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, query,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get funding loans history", v, sc) }
    
    arr := FastjsonGetArray(v)
    loansLen := len(arr)
    loans := make([]Loan, loansLen)
    
    for i, v := range arr {
        bitfinexGetLoanFromJson(v, &loans[i])
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
    credit.Amount = FastjsonGetUDec128(arr[5], 8)
    credit.Status = FastjsonGetString(arr[7])
    credit.Rate = FastjsonGetUDec128(arr[11], 8)
    credit.Period = FastjsonGetUInt32(arr[12])
    credit.Renew = FastjsonGetUInt32(arr[18])!=0
    credit.NoClose = FastjsonGetUInt32(arr[20])!=0
    credit.Market = FastjsonGetString(arr[21])[1:]
}

func (drv *BitfinexPrivate) GetFundingCredits(currency string) []Credit {
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

func (drv *BitfinexPrivate) GetFundingCreditsHistory(currency string,
                                since time.Time, limit uint) []Credit {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingCredits...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/hist"...)
    query := make([]byte, 0, 40)
    query = append(query, "?limit="...)
    query = strconv.AppendUint(query, uint64(limit), 10)
    if !since.IsZero() {
        unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
        query = append(query, "&start="...)
        query = strconv.AppendInt(query, unixTime, 10)
    }
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, query,
                                    bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get funding credits history", v, sc) }
    
    arr := FastjsonGetArray(v)
    creditsLen := len(arr)
    credits := make([]Credit, creditsLen)
    
    for i, v := range arr {
        bitfinexGetCreditFromJson(v, &credits[i])
    }
    return credits
}

func (drv *BitfinexPrivate) SubmitBidOrder(currency string, amount,
                                    rate godec128.UDec128, period uint32) {
    body := make([]byte, 0, 80)
    body = append(body, `{"type":"LIMIT","symbol":"f`...)
    body = append(body, currency...)
    body = append(body, `","amount":"-`...)
    body = append(body, amount.FormatBytes(8, false)...)
    body = append(body, `","rate":"`...)
    body = append(body, rate.FormatBytes(8, false)...)
    body = append(body, `","period":`...)
    body = strconv.AppendUint(body, uint64(period), 10)
    body = append(body, `,"flags":0}`...)
    fmt.Println("body:", string(body))
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost,
                                    bitfinexApiSubmit, nil, body)
    if sc >= 400 { bitfinexPanic("Can't submit order", v, sc) }
    fmt.Println("response:", v)
}
