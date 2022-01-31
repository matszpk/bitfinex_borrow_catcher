/*
 * engine_test.go - main engine module
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
    "time"
    "github.com/matszpk/godec64"
    "testing"
)

func getTestEngine0() *Engine {
    return &Engine{
        baseCurrMarkets: map[string]bool{
            "USTUSD": true },
        quoteCurrMarkets: map[string]bool{
            "BTCUST": true, "ADAUST": true },
        config: &Config{
            Currency: "UST", AutoLoanFetchPeriod: 20*time.Minute,
            AutoLoanFetchShift: 15*time.Minute,
            AutoLoanFetchEndShift: 9*time.Minute + 20*time.Second,
            MinRateDifference: 0.2,
            MinOrderAmount: 150, },
    }
}

func TestCalculateTotalBorrow(t *testing.T) {
    eng := getTestEngine0()
    poss := []Position{
        Position{ Market: "BTCUST", Amount: 155000000,
            BasePrice: 211000000000, Long: true },
        Position{ Market: "BTCUSD", Amount: 452000000,
            BasePrice: 661000000000, Long: true },
        Position{ Market: "ADAUST", Amount: 1355000000,
            BasePrice: 140000000000, Long: true },
        Position{ Market: "USTUSD", Amount: 2334000000,
            BasePrice: 99100000, Long: false } }
    bals := []Balance{
        Balance{ Currency: "UST", Total: 120000000 },
        Balance{ Currency: "USD", Total: 11100000000 },
    }
    expTotBorrow := godec64.UDec64(2226264000000)
    resTotBorrow := eng.calculateTotalBorrow(poss, bals)
    if expTotBorrow != resTotBorrow {
        t.Errorf("TotBorrow mismatch: %v!=%v", expTotBorrow, resTotBorrow)
    }
    
    expTotBorrow = godec64.UDec64(2226384000000)
    resTotBorrow = eng.calculateTotalBorrow(poss, nil)
    if expTotBorrow != resTotBorrow {
        t.Errorf("TotBorrow mismatch: %v!=%v", expTotBorrow, resTotBorrow)
    }
}
