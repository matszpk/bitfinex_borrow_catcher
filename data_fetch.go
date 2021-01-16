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
const maxPeriodUpdate = 30

type DataFetcher struct {
    mutex sync.Mutex
    currency string
    public *BitfinexPublic
    rtPublic *BitfinexRTPublic
    marketPriceLastUpdate int64         // atomic
    orderBookLastUpdate int64         // atomic
    rtLastUpdate int64     // atomic
    marketPrice atomic.Value
    orderBook atomic.Value
    lastTrades atomic.Value
}

func NewDataFetcher(public *BitfinexPublic, rtPublic *BitfinexRTPublic,
                    currency string) *DataFetcher {
    df := &DataFetcher{ currency: currency, public: public, rtPublic: rtPublic,
        marketPriceLastUpdate: 0, orderBookLastUpdate: 0,
        rtLastUpdate: 0 }
    rtPublic.SubscribeMarketPrice(currency, df.marketPriceHandler)
    rtPublic.SubscribeOrderBook(currency, df.orderBookHandler)
    return df
}

func (df *DataFetcher) marketPriceHandler(mp godec128.UDec128) {
    df.marketPrice.Store(mp)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
}

func (df *DataFetcher) orderBookHandler(ob *OrderBook) {
    var newOb OrderBook
    newOb.copyFrom(ob)        // copy to avoid problems
    df.orderBook.Store(&newOb)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
}

func (df *DataFetcher) GetMarkerPrice() godec128.UDec128 {
    t := time.Now().Unix()
    if t - atomic.LoadInt64(&df.rtLastUpdate) >= maxRtPeriodUpdate &&
        t - atomic.LoadInt64(&df.marketPriceLastUpdate) >= maxPeriodUpdate {
        // get from HTTP
        mp := df.public.GetMarketPrice(df.currency)
        df.marketPrice.Store(mp)
        atomic.StoreInt64(&df.marketPriceLastUpdate, t)
        return mp
    }
    return df.marketPrice.Load().(godec128.UDec128)
}

func (df *DataFetcher) GetOrderBook() *OrderBook {
    t := time.Now().Unix()
    if t - atomic.LoadInt64(&df.rtLastUpdate) >= maxRtPeriodUpdate &&
        t - atomic.LoadInt64(&df.orderBookLastUpdate) >= maxPeriodUpdate {
        // get from HTTP
        var ob OrderBook
        df.public.GetOrderBook(df.currency, &ob)
        df.orderBook.Store(&ob)
        atomic.StoreInt64(&df.orderBookLastUpdate, t)
        return &ob
    }
    return df.orderBook.Load().(*OrderBook)
}

func (df *DataFetcher) GetLastTrades() []Trade {
    return nil
}
