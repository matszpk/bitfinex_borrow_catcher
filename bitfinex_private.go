/*
 * bitfinex_private.go - Bitfinex Private client
 *
 * bitfinex_funding_catcher - Automatic funding catcher for open positions in
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
                host, uri []byte, bodyStr []byte) (*fastjson.Value, int) {
    nonceB := strconv.AppendInt(nil ,time.Now().UnixNano()/100000, 10)
    // generate signature
    sig := make([]byte, 0, 200)
    sig = append(sig, bitfinexStrApiPrefix...)
    sig = append(sig, uri...)
    sig = append(sig, nonceB...)
    sig = append(sig, bodyStr...)
    
    sumGen := hmac.New(sha512.New384, drv.apiSecret)
    if _, err := sumGen.Write(sig); err!=nil {
        ErrorPanic("Arrgghh:", err)
    }
    sum := sumGen.Sum(nil)
    sumHex := make([]byte, len(sum)*2)
    hex.Encode(sumHex, sum)
    
    headers := [][]byte{
        bitfinexStrNonce, nonceB,
        bitfinexStrApiKey, drv.apiKey,
        bitfinexStrSignature, sumHex }
    
    return rh.HandleHttpPostJson(drv.httpClient, host, uri, bodyStr, headers)
}

func bitfinexGetLoanFromJson(v *fastjson.Value, loan *Loan) {
    arr := FastjsonGetArray(v)
    if len(arr) < 22 {
        panic("Wrong json body")
    }
    *loan = Loan{}
    loan.Id = FastjsonGetUInt64(arr[0])
    loan.Currency = FastjsonGetString(arr[1])
    loan.Side = FastjsonGetInt(arr[2])
    loan.CreateTime = FastjsonGetUnixTimeMilli(arr[3])
    loan.UpdateTime = FastjsonGetUnixTimeMilli(arr[4])
    loan.Amount = FastjsonGetUDec128(arr[5], 8)
    loan.Status = FastjsonGetString(arr[7])
    loan.Rate = FastjsonGetUDec128(arr[11], 8)
    loan.Period = FastjsonGetUInt32(arr[14])
    loan.Renew = FastjsonGetUInt32(arr[19])!=0
    loan.NoClose = FastjsonGetUInt32(arr[21])!=0
}

func (drv *BitfinexPrivate) GetFundingLoans(currency string) []Loan {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiFundingLoans...)
    apiUrl = append(apiUrl, currency...)
        
    var rh RequestHandle
    defer rh.Release()
    v, sc := drv.handleHttpPostJson(&rh, bitfinexPrivApiHost, apiUrl, bitfinexStrEmptyJson)
    if sc >= 400 { bitfinexPanic("Can't get funding loans", v, sc) }
    
    arr := FastjsonGetArray(v)
    
    loansLen := len(arr)
    loans := make([]Loan, loansLen)
    
    for i, v := range arr {
        bitfinexGetLoanFromJson(v, &loans[i])
    }
    return loans
}
