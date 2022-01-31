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
            MinRateDifference: 0.2, MinOrderAmount: 150 },
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

func equalBorrowTask(a, b *BorrowTask) bool {
    if a.TotalBorrow != b.TotalBorrow { return false }
    if len(a.LoanIdsToClose) != len(b.LoanIdsToClose) { return false }
    for i := 0; i < len(a.LoanIdsToClose); i++ {
        if a.LoanIdsToClose[i] != b.LoanIdsToClose[i] { return false }
    }
    return true
}

func TestPrepareBorrowTask(t *testing.T) {
    eng := getTestEngine0()
    now := time.Date(2021, 9, 14, 15, 37, 11, 0, time.UTC)
    ob := OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 4111000000 },
            OrderBookEntry{ 11, 3, 20200000000, 4112000000 },
            OrderBookEntry{ 12, 2, 134177000000, 4115000000 },
            OrderBookEntry{ 13, 2, 53400000000, 4118000000 },
            OrderBookEntry{ 14, 2, 78800000000, 4125000000 },
        },
    }
    var totalCredits godec64.UDec64
    credits := []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 32455000000, Status: "ACTIVE",
                Rate: 7321000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 2441355000000, Status: "ACTIVE",
                Rate: 6663000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 141355000000, Status: "ACTIVE",
                Rate: 8934000000, Period: 2 }, "ADAUST" } }
    for i := 0; i < len(credits); i++ {
        totalCredits += credits[i].Amount
    }
    resTask := eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask := BorrowTask{ 173810000000, []uint64{ 102, 100 } }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
}
