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
    "os"
    "os/signal"
    "syscall"
)

func main() {
    var config Config
    signal.Ignore(syscall.SIGHUP)
    config.Load("bbc_config.json")
    Logger.SetOutput(os.Stderr)
    Logger.SetLevel("info")
    
    if len(os.Args) >= 3 && os.Args[1] == "genpassword" {
        GenPassword(os.Args[2])
        return
    }
    
    apiKey, secretKey := AuthenticateExchange(&config)
    
    bp := NewBitfinexPublic()
    bprt := NewBitfinexRTPublic()
    bprt.Start()
    defer bprt.Stop()
    bpriv := NewBitfinexPrivate(apiKey, secretKey)
    df := NewDataFetcher(bp, bprt, config.Currency)
    df.Start()
    defer df.Stop()
    
    eng := NewEngine(&config, df, bpriv)
    eng.Start()
    defer eng.Stop()
    
    select{}
}
