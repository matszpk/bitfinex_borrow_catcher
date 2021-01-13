/*
 * bitfinex_public.go - Bitfinex Public client
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
    "fmt"
    "strconv"
    "time"
    "github.com/matszpk/godec128"
    "github.com/valyala/fasthttp"
    "github.com/valyala/fastjson"
)

var (
    bitfinexPubApiHost = []byte("api-pub.bitfinex.com")
    bitfinexApiTrades = []byte("/v2/trades/f")
    bitfinexApiOrderBook = []byte("/v2/book/f")
    bitfinexApiCandles = []byte("/v2/candles/trade:")
)

type Side uint8

// Side of order
const (
    SideBid Side = iota
    SideOffer
)

type Trade struct {
    Id uint64
    TimeStamp time.Time
    Side Side
    Amount godec128.UDec128
    Rate godec128.UDec128
    Period uint32
}

type OrderBookEntry struct {
    Id uint64
    Period uint32
    Amount godec128.UDec128
    Rate godec128.UDec128
}

type OrderBook struct {
    Bid []OrderBookEntry
    Ask []OrderBookEntry
}

// Candle structure
type Candle struct {
    TimeStamp time.Time     /// timestamp
    // Open, Low, High, Close in rate
    Open, High, Low, Close godec128.UDec128
    // volume in currency
    Volume godec128.UDec128
}

type BitfinexPublic struct {
    httpClient fasthttp.HostClient
}

func NewBitfinexPublic() *BitfinexPublic {
    return &BitfinexPublic{ httpClient: fasthttp.HostClient{
        Addr: "api.bitfinex.com,api-pub.bitfinex.com",
        IsTLS: true, ReadTimeout: time.Second*60 } }
}

func bitfinexPanic(msg string, v *fastjson.Value, sc int) {
    if v!=nil {
        switch v.Type() {
            case fastjson.TypeArray: {
                arr := FastjsonGetArray(v)
                first := FastjsonGetString(arr[0])
                if len(arr)!=0 && first=="error" {
                    code := FastjsonGetUInt64(arr[1])
                    var errMsg string
                    if len(arr) > 2 {
                        errMsg = FastjsonGetString(arr[2])
                    }
                    panic(fmt.Sprint(msg, ": ", code, " ", errMsg))
                }
            }
            case fastjson.TypeObject: {
                errMsg := string(v.GetStringBytes("message"))
                panic(fmt.Sprint(msg, ": ", errMsg))
            }
        }
    }
    HttpPanic(msg, sc)
}

func bitfinexGetTradeFromJson(v *fastjson.Value, trade *Trade) {
    arr := FastjsonGetArray(v)
    if len(arr) < 5 {
        panic("Wrong json body")
    }
    trade.Id = FastjsonGetUInt64(arr[0])
    trade.TimeStamp = FastjsonGetUnixTimeMilli(arr[1])
    var neg bool
    trade.Side = SideBid
    trade.Amount, neg = FastjsonGetUDec128Signed(arr[2], 8)
    if neg {
        trade.Side = SideOffer
    }
    trade.Rate = FastjsonGetUDec128(arr[3], 8)
    trade.Period = FastjsonGetUInt32(arr[4])
}

//
func (drv *BitfinexPublic) GetTrades(currency string,
                            since time.Time, limit uint) []Trade {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiTrades...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/hist?limit="...)
    apiUrl = strconv.AppendUint(apiUrl, uint64(limit), 10)
    if !since.IsZero() {
        unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
        apiUrl = append(apiUrl, "&start="...)
        apiUrl = strconv.AppendInt(apiUrl, unixTime, 10)
    }
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get trades", v, sc) }
    arr := FastjsonGetArray(v)
    
    tradesLen := len(arr)
    trades := make([]Trade, tradesLen)
    for i, v := range arr {
        bitfinexGetTradeFromJson(v, &trades[tradesLen-i-1])
    }
    return trades
}

func bitfinexGetOrderBookEntryFromJson(v *fastjson.Value, obe *OrderBookEntry) bool {
    arr := FastjsonGetArray(v)
    if len(arr) < 3 {
        panic("Wrong json body")
    }
    obe.Id = FastjsonGetUInt64(arr[0])
    obe.Period = FastjsonGetUInt32(arr[1])
    obe.Rate = FastjsonGetUDec128(arr[2], 8)
    var neg bool
    obe.Amount, neg = FastjsonGetUDec128Signed(arr[3], 8)
    return neg
}

func bitfinexGetOrderBookFromJson(v *fastjson.Value, ob *OrderBook) {
    arr := FastjsonGetArray(v)
    
    arrLen := len(arr)
    ob.Bid = make([]OrderBookEntry, 0, arrLen>>1)
    ob.Ask = make([]OrderBookEntry, 0, arrLen>>1)
    
    var obe OrderBookEntry
    // orderbook entries is in correct order
    for _, obev := range arr {
        if bitfinexGetOrderBookEntryFromJson(obev, &obe) {
            ob.Bid = append(ob.Bid, obe)
        } else {
            ob.Ask = append(ob.Ask, obe)
        }
    }
}

func (drv *BitfinexPublic) GetOrderBook(currency string, ob *OrderBook) {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiOrderBook...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/R0?len=100"...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get orderbook", v, sc) }
    bitfinexGetOrderBookFromJson(v, ob)
}

func bitfinexCandlePeriodString(period uint32) string {
    periodStr := ""
    switch period {
        case 60: periodStr = "1m"
        case 5*60: periodStr = "5m"
        case 15*60: periodStr = "15m"
        case 30*60: periodStr = "30m"
        case 3600: periodStr = "1h"
        case 3*3600: periodStr = "3h"
        case 6*3600: periodStr = "6h"
        case 12*3600: periodStr = "12h"
        case 24*3600: periodStr = "1D"
        case 7*24*3600: periodStr = "7D"
        case 14*24*3600: periodStr = "14D"
        case 30*24*3600: periodStr = "1M"
        default:
            panic("Unsupported candle period")
    }
    return periodStr
}

func bitfinexGetCandleFromJson(v *fastjson.Value, candle *Candle) {
    arr := FastjsonGetArray(v)
    if len(arr) < 6 {
        panic("Wrong json body")
    }
    candle.TimeStamp = FastjsonGetUnixTimeMilli(arr[0])
    candle.Open = FastjsonGetUDec128(arr[1], 8)
    candle.Close = FastjsonGetUDec128(arr[2], 8)
    candle.High = FastjsonGetUDec128(arr[3], 8)
    candle.Low = FastjsonGetUDec128(arr[4], 8)
    candle.Volume = FastjsonGetUDec128(arr[5], 8)
}

func (drv *BitfinexPublic) GetCandles(currency string, period uint32,
                            since time.Time, limit uint) []Candle {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiCandles...)
    apiUrl = append(apiUrl, bitfinexCandlePeriodString(period)...)
    apiUrl = append(apiUrl, ":f"...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, ":a30:p2:p30/hist?sort=1&start="...)
    if since.IsZero() {
        since = time.Now().Add(-time.Duration(limit) *
                        time.Duration(period) * time.Second)
    }
    unixTime := since.Unix()*1000 + int64(since.Nanosecond()/1000000)
    apiUrl = strconv.AppendInt(apiUrl, unixTime, 10)
    apiUrl = append(apiUrl, "&limit="...)
    apiUrl = strconv.AppendUint(apiUrl, uint64(limit), 10)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get candles", v, sc) }
    
    arr := FastjsonGetArray(v)
    candles := make([]Candle, len(arr))
    
    for i, cv := range arr {
        bitfinexGetCandleFromJson(cv, &candles[i])
    }
    return candles
}
