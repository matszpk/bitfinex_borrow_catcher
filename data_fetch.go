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

const maxRtPeriodUpdate = time.Minute*5

type DataFetcher struct {
    mutex sync.Mutex
    public *BitfinexPublic
    rtPublic *BitfinexRTPublic
    lastUpdate int64         // atomic
    rtLastUpdate int64     // atomic
    marketPrice atomic.Value
    orderBook atomic.Value
    lastTrades atomic.Value
}

func NewDataFetcher(public *BitfinexPublic, rtPublic *BitfinexRTPublic,
                    currency string) *DataFetcher {
    df := &DataFetcher{ public: public, rtPublic: rtPublic,
        lastUpdate: 0, rtLastUpdate: 0 }
    rtPublic.SubscribeMarketPrice(currency, df.marketPriceHandler)
    rtPublic.SubscribeOrderBook(currency, df.orderBookHandler)
    return df
}

func (df *DataFetcher) marketPriceHandler(mp godec128.UDec128) {
    df.marketPrice.Store(mp)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
}

func (df *DataFetcher) orderBookHandler(ob *OrderBook) {
    df.orderBook.Store(ob)
    atomic.StoreInt64(&df.rtLastUpdate, time.Now().Unix())
}

func (df *DataFetcher) GetMarkerPrice() godec128.UDec128 {
    return df.marketPrice.Load().(godec128.UDec128)
}

func (df *DataFetcher) GetOrderBook() *OrderBook {
    return df.orderBook.Load().(*OrderBook)
}

func (df *DataFetcher) GetLastTrades() []Trade {
    return nil
}
