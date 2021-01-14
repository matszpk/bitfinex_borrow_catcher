/*
 * main.go - main program
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
    //"time"
    "github.com/chzyer/readline"
    //"github.com/matszpk/godec128"
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
    //fmt.Println("APIKey:", string(apiKey))
    //fmt.Println("SecretKey:", string(secretKey))
    bpriv := NewBitfinexPrivate(apiKey, secretKey)
    for _, l := range bpriv.GetFundingLoans("USD") {
        fmt.Println(l)
    }
}
