/*
 * bitfinex_public.go - Bitfinex Public client
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
    "fmt"
    "sort"
    "strconv"
    "strings"
    "time"
    "github.com/matszpk/godec64"
    "github.com/valyala/fasthttp"
    "github.com/valyala/fastjson"
)

var (
    bitfinexPubApiHost = []byte("api-pub.bitfinex.com")
    bitfinexApiTrades = []byte("/v2/trades/f")
    bitfinexApiOrderBook = []byte("/v2/book/f")
    bitfinexApiCandles = []byte("/v2/candles/trade:")
    bitfinexApiMarkets = []byte("v2/conf/pub:list:pair:exchange")
    bitfinexApiTicker = []byte("/v2/ticker/t")
)

// About rate: interest rate in percent is multiplied by 10000000000

type Side uint8

// Side of order
const (
    SideBid Side = iota
    SideOffer
)

type Market struct {
    Name string
    BaseCurrency string
    QuoteCurrency string
}

type Trade struct {
    Id uint64
    TimeStamp time.Time
    Side Side
    Amount godec64.UDec64
    Rate godec64.UDec64
    Period uint32
}

type OrderBookEntry struct {
    Period uint32
    Amount godec64.UDec64
    Rate godec64.UDec64
}

func (obe *OrderBookEntry) Cmp(obe2 *OrderBookEntry) int {
    if obe.Rate < obe2.Rate { return -1
    } else if obe.Rate > obe2.Rate { return 1 }
    /*if obe.Id < obe2.Id {
        return -1
    } else if obe.Id > obe2.Id {
        return 1
    }*/
    return 0
}

type OrderBookEntrySorter []OrderBookEntry

func (s OrderBookEntrySorter) Len() int {
    return len(s)
}

func (s OrderBookEntrySorter) Less(i, j int) bool {
    return s[i].Cmp(&s[j]) < 0
}

func (s OrderBookEntrySorter) Swap(i, j int) {
    t := s[i]
    s[i] = s[j]
    s[j] = t
}

type OrderBookEntryRevSorter []OrderBookEntry

func (s OrderBookEntryRevSorter) Len() int {
    return len(s)
}

func (s OrderBookEntryRevSorter) Less(i, j int) bool {
    return s[i].Cmp(&s[j]) > 0
}

func (s OrderBookEntryRevSorter) Swap(i, j int) {
    t := s[i]
    s[i] = s[j]
    s[j] = t
}

type OrderBook struct {
    Bid []OrderBookEntry
    Ask []OrderBookEntry
}

func (ob *OrderBook) copyFrom(src *OrderBook) {
    blen, alen := len(src.Bid), len(src.Ask)
    ob.Bid = ob.Bid[:0]
    ob.Ask = ob.Ask[:0]
    ob.Bid = append(ob.Bid, src.Bid[:blen]...)
    ob.Ask = append(ob.Ask, src.Ask[:alen]...)
}

// Candle structure
type Candle struct {
    TimeStamp time.Time     /// timestamp
    // Open, Low, High, Close in rate
    Open, High, Low, Close godec64.UDec64
    // volume in currency
    Volume godec64.UDec64
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

func bitfinexGetMarketsFromJson(v *fastjson.Value, market *Market) {
    market.Name = FastjsonGetString(v)
    if colonIdx := strings.IndexRune(market.Name, ':'); colonIdx>=0 {
        market.BaseCurrency = market.Name[:colonIdx]
        market.QuoteCurrency = market.Name[colonIdx+1:]
    } else if len(market.Name)>3 {
        mlen := len(market.Name)
        market.BaseCurrency = market.Name[:mlen-3]
        market.QuoteCurrency = market.Name[mlen-3:]
    } else {
        panic("Wrong market name")
    }
}

func (drv *BitfinexPublic) GetMarkets() []Market {
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost,
                                  bitfinexApiMarkets, nil)
    if sc >= 400 { bitfinexPanic("Can't get markets", v, sc) }
    arr := FastjsonGetArray(v)
    if len(arr) < 1 {
        panic("Wrong json body")
    }
    arr = FastjsonGetArray(arr[0])
    marketsLen := len(arr)
    markets := make([]Market, marketsLen)
    for i, v := range arr {
        bitfinexGetMarketsFromJson(v, &markets[i])
    }
    return markets
}

func bitfinexGetMarketPriceFromJson(v *fastjson.Value) godec64.UDec64 {
    arr := FastjsonGetArray(v)
    if len(arr) < 7 {
        panic("Wrong json body")
    }
    return FastjsonGetUDec64(arr[6], 8)
}

func (drv *BitfinexPublic) GetMarketPrice(market string) godec64.UDec64 {
    apiUrl := make([]byte, 0, 20)
    apiUrl = append(apiUrl, bitfinexApiTicker...)
    apiUrl = append(apiUrl, market...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get ticker", v, sc) }
    
    return bitfinexGetMarketPriceFromJson(v)
}


func bitfinexGetTradeFromJson(v *fastjson.Value, trade *Trade) {
    arr := FastjsonGetArray(v)
    if len(arr) < 5 {
        panic("Wrong json body")
    }
    trade.Id = FastjsonGetUInt64(arr[0])
    trade.TimeStamp = FastjsonGetUnixTimeMilli(arr[1])
    var neg bool
    trade.Side = SideOffer
    trade.Amount, neg = FastjsonGetUDec64Signed(arr[2], 8)
    if neg {
        trade.Side = SideBid
    }
    trade.Rate = FastjsonGetUDec64(arr[3], 12)
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
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
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
    obe.Period = FastjsonGetUInt32(arr[1])
    obe.Rate = FastjsonGetUDec64(arr[0], 12)
    var neg bool
    obe.Amount, neg = FastjsonGetUDec64Signed(arr[3], 8)
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
    sort.Sort(OrderBookEntryRevSorter(ob.Bid))
    sort.Sort(OrderBookEntrySorter(ob.Ask))
}

func (drv *BitfinexPublic) GetOrderBook(currency string, ob *OrderBook) {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiOrderBook...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/P0?len=25"...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get orderbook", v, sc) }
    bitfinexGetOrderBookFromJson(v, ob)
}

func (drv *BitfinexPublic) GetMaxOrderBook(currency string, ob *OrderBook) {
    apiUrl := make([]byte, 0, 60)
    apiUrl = append(apiUrl, bitfinexApiOrderBook...)
    apiUrl = append(apiUrl, currency...)
    apiUrl = append(apiUrl, "/P0?len=100"...)
    
    var rh RequestHandle
    defer rh.Release()
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
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
    candle.Open = FastjsonGetUDec64(arr[1], 12)
    candle.Close = FastjsonGetUDec64(arr[2], 12)
    candle.High = FastjsonGetUDec64(arr[3], 12)
    candle.Low = FastjsonGetUDec64(arr[4], 12)
    candle.Volume = FastjsonGetUDec64(arr[5], 12)
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
    v, sc := rh.HandleHttpGetJson(&drv.httpClient, bitfinexPubApiHost, apiUrl, nil)
    if sc >= 400 { bitfinexPanic("Can't get candles", v, sc) }
    
    arr := FastjsonGetArray(v)
    candles := make([]Candle, len(arr))
    
    for i, cv := range arr {
        bitfinexGetCandleFromJson(cv, &candles[i])
    }
    return candles
}
