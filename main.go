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
    "github.com/matszpk/godec128"
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
    /*apiKey, secretKey := Authenticate()
    bpriv := NewBitfinexPrivate(apiKey, secretKey)
    var res OpResult
    bpriv.SubmitBidOrder("USD", godec128.UDec128{51*100000000,0},
                      godec128.UDec128{1233,0}, 2, &res)
    fmt.Println(res)
    time.Sleep(5*time.Second)
    fmt.Println("Orders:")
    for _, order := range bpriv.GetActiveOrders("USD") {
        fmt.Println(order)
    }
    time.Sleep(25*time.Second)
    fmt.Println("Canceling order:", res.Order.Id)
    bpriv.CancelOrder(res.Order.Id, &res)
    fmt.Println(res)*/
    /*bp := NewBitfinexPublic()
    fmt.Println("BTCUSD", bp.GetMarketPrice("ADAUSD").Format(8, false))*/
    /*bprt := NewBitfinexRTPublic()
    bprt.Start()
    defer bprt.Stop()
    bprt.SubscribeTrades("USD", func(t *Trade) {
        fmt.Println(*t)
    })
    time.Sleep(time.Minute*10)*/
    bp := NewBitfinexPublic()
    bprt := NewBitfinexRTPublic()
    bprt.Start()
    defer bprt.Stop()
    df := NewDataFetcher(bp, bprt, "USD")
    df.SetUSDPriceHandler(func(mp godec128.UDec128) {
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
    for i:=0; i < 500; i++ {
        fmt.Println("USD Status:")
        fmt.Println("USD USD Price:", df.GetUSDPrice().Format(8, false))
        ob := df.GetOrderBook()
        fmt.Println("USD Funding Bid:", len(ob.Bid))
        fmt.Println("USD Funding Ask:", len(ob.Ask))
        fmt.Println("USD Funding Trade:", *df.GetLastTrade())
        time.Sleep(5*time.Second)
    }
}
