/*
 * data_fetch.go - data fetching module
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
    "sync"
    "sync/atomic"
    "time"
    "github.com/matszpk/godec128"
)

const maxRtPeriodUpdate = 60*5
const maxPeriodUpdate = 10
const dfUpdaterPeriod = time.Second*10

var usdMarketsOnce sync.Once
var usdMarkets map[string]Market

func initUSDMarkets() {
    bp := NewBitfinexPublic()
    markets := bp.GetMarkets()
    
    usdMarkets = make(map[string]Market)
    for _, m := range markets {
        if m.QuoteCurrency == "USD" || m.QuoteCurrency == "UST" {
            // insert if entry is empty or if quote currency is USD
            if _, ok := usdMarkets[m.BaseCurrency]; !ok || m.QuoteCurrency=="USD" {
                usdMarkets[m.BaseCurrency] = m // 
            }
        }
    }
}

type DataFetcher struct {
    stopCh chan struct{}
    usdFiat bool
    noUsdPrice bool
    currency string
    public *BitfinexPublic
    rtPublic *BitfinexRTPublic
    marketPriceLastUpdate int64         // atomic
    orderBookLastUpdate int64         // atomic
    tradeLastUpdate int64
    rtLastUpdate int64     // atomic
    marketPrice atomic.Value
    orderBook atomic.Value
    lastTrade atomic.Value
    tradeHandler TradeHandler
    marketPriceHandlerU MarketPriceHandler
    orderBookHandlerU OrderBookHandler
    lastTradeHandlerU TradeHandler
}

func NewDataFetcher(public *BitfinexPublic, rtPublic *BitfinexRTPublic,
                    currency string) *DataFetcher {
    usdMarketsOnce.Do(initUSDMarkets)
    
    df := &DataFetcher{ stopCh: make(chan struct{}),
        usdFiat: false, noUsdPrice: false,
        currency: currency, public: public, rtPublic: rtPublic,
        marketPriceLastUpdate: 0, orderBookLastUpdate: 0,
        rtLastUpdate: 0 }
    
    if currency!="USD" && currency!="UST" {
        if _, ok := usdMarkets[currency]; ok {
            df.usdFiat = false
        } else {
            df.noUsdPrice = true
        }
    } else {
        df.usdFiat = true
    }
    
    if !df.noUsdPrice && !df.usdFiat {
        rtPublic.SubscribeMarketPrice(usdMarkets[df.currency].Name, df.marketPriceHandler)
    }
    rtPublic.SubscribeOrderBook(currency, df.orderBookHandler)
    return df
}

func (df *DataFetcher) SetUSDPriceHandler(mh MarketPriceHandler) {
    df.marketPriceHandlerU = mh
}

func (df *DataFetcher) SetOrderBookHandler(oh OrderBookHandler) {
    df.orderBookHandlerU = oh
}

func (df *DataFetcher) SetLastTradeHandler(th TradeHandler) {
    df.lastTradeHandlerU = th
}

func (df *DataFetcher) Start() {
    df.marketPrice.Store(godec128.UDec128{})
    df.orderBook.Store(&OrderBook{})
    df.lastTrade.Store(&Trade{})
    go df.updater()
}

func (df *DataFetcher) Stop() {
    df.stopCh <- struct{}{}
}

func (df *DataFetcher) update() {
    t := time.Now().Unix()
    needUpdate := t - atomic.LoadInt64(&df.rtLastUpdate) >= maxRtPeriodUpdate
    
    mpObj := df.marketPrice.Load()
    if !df.usdFiat && !df.noUsdPrice && (needUpdate || mpObj==nil) {
        // get from HTTP
        mp := df.public.GetMarketPrice(usdMarkets[df.currency].Name)
        df.marketPrice.Store(mp)
        atomic.StoreInt64(&df.marketPriceLastUpdate, t)
        if df.marketPriceHandlerU!=nil {
            df.marketPriceHandlerU(mp)
        }
    }
    
    obObj := df.orderBook.Load()
    if needUpdate || obObj==nil {
        // get from HTTP
        var ob OrderBook
        df.public.GetOrderBook(df.currency, &ob)
        df.orderBook.Store(&ob)
        atomic.StoreInt64(&df.orderBookLastUpdate, t)
        if df.orderBookHandlerU!=nil {
            df.orderBookHandlerU(&ob)
        }
    }
    
    // get from HTTP
    trades := df.public.GetTrades(df.currency, time.Time{}, 1)
    atomic.StoreInt64(&df.tradeLastUpdate, t)
    if len(trades)!=0 {
        df.lastTrade.Store(&trades[0])
        if df.lastTradeHandlerU!=nil {
            df.lastTradeHandlerU(&trades[0])
        }
    } else {
        df.lastTrade.Store(&Trade{})
    }
}

func (df *DataFetcher) safeUpdate() {
    defer func() {
        if x := recover(); x!=nil {
            Logger.Error("Error while DataFetcher updating: ", x)
        }
    }()
    df.update()
}

func (df *DataFetcher) updater() {
    ticker := time.NewTicker(dfUpdaterPeriod)
    defer ticker.Stop()
    
    df.safeUpdate()
    stopped := false
    for !stopped {
        select {
            case <- ticker.C:
                df.safeUpdate()
            case <- df.stopCh:
                stopped = true
        }
    }
}

func (df *DataFetcher) IsUSDPrice() bool {
    return !df.noUsdPrice
}

func (df *DataFetcher) marketPriceHandler(mp godec128.UDec128) {
    df.marketPrice.Store(mp)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
    if df.marketPriceHandlerU!=nil {
        df.marketPriceHandlerU(mp)
    }
}

func (df *DataFetcher) orderBookHandler(ob *OrderBook) {
    var newOb OrderBook
    newOb.copyFrom(ob)        // copy to avoid problems
    df.orderBook.Store(&newOb)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
    if df.orderBookHandlerU!=nil {
        df.orderBookHandlerU(&newOb)
    }
}

func (df *DataFetcher) GetUSDPrice() godec128.UDec128 {
    if df.usdFiat {
        return godec128.UDec128{ 100000000, 0 }
    }
    if df.noUsdPrice {
        panic("No USD Price")
    }
    return df.marketPrice.Load().(godec128.UDec128)
}

func (df *DataFetcher) GetOrderBook() *OrderBook {
    return df.orderBook.Load().(*OrderBook)
}

func (df *DataFetcher) GetLastTrade() *Trade {
    return df.lastTrade.Load().(*Trade)
}
