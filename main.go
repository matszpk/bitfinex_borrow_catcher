/*
 * main.go - main program
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
    "time"
    "github.com/chzyer/readline"
    //"github.com/matszpk/godec64"
)

func Authenticate() ([]byte, []byte) {
    apiKey, err := readline.Password("Enter APIKey:")
    if err!=nil {
        panic(fmt.Sprint("Can't read APIKey: ", err))
    }
    secretKey, err := readline.Password("Enter SecretKey:")
    if err!=nil {
        panic(fmt.Sprint("Can't read SecretKey: ", err))
    }
    return apiKey, secretKey
}

func main() {
    apiKey, secretKey := Authenticate()
    bpriv := NewBitfinexPrivate(apiKey, secretKey)
    for _, credit := range bpriv.GetLoansHistory("UST", time.Time{}, 100) {
        fmt.Println(credit)
    }
    /*bp := NewBitfinexPublic()
    fmt.Println("BTCUSD", bp.GetMarketPrice("ADAUSD").Format(8, false))*/
    /*bprt := NewBitfinexRTPublic()
    bprt.Start()
    defer bprt.Stop()
    bprt.SubscribeOrderBook("USD", func(ob *OrderBook) {
        fmt.Println("MyOB:", len(ob.Bid), len(ob.Ask))
    })
    bprt.SubscribeTrades("USD", func(tr *Trade) {
        fmt.Println("MyTrade:", *tr)
    })
    time.Sleep(time.Minute*10)*/
    /*bp := NewBitfinexPublic()
    bprt := NewBitfinexRTPublic()
    bprt.Start()
    defer bprt.Stop()
    df := NewDataFetcher(bp, bprt, "LTC")
    df.SetUSDPriceHandler(func(mp godec64.UDec64) {
        fmt.Println("MyPrice:", mp.Format(8, false))
    })
    df.SetOrderBookHandler(func(ob *OrderBook) {
        fmt.Println("MyOB:", len(ob.Bid), len(ob.Ask))
    })
    df.SetLastTradeHandler(func(tr *Trade) {
        fmt.Println("MyLastTrade:", *tr)
    })
    df.Start()
    defer df.Stop()
    for i:=0; i < 1000; i++ {
        fmt.Println("LTC Status:")
        fmt.Println("LTC LTC Price:", df.GetUSDPrice().Format(8, false))
        ob := df.GetOrderBook()
        fmt.Println("LTC Funding Bid:", ob.Bid)
        fmt.Println("LTC Funding Ask:", ob.Ask)
        fmt.Println("LTC Funding Trade:", *df.GetLastTrade())
        time.Sleep(5*time.Second)
    }*/
}
