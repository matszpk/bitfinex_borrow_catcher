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
    if a.Rate != b.Rate { return false }
    if len(a.LoanIdsToClose) != len(b.LoanIdsToClose) { return false }
    for i := 0; i < len(a.LoanIdsToClose); i++ {
        if a.LoanIdsToClose[i] != b.LoanIdsToClose[i] { return false }
    }
    return true
}

func sumTotalCredits(credits []Credit) godec64.UDec64 {
    var totalCredits godec64.UDec64
    for i := 0; i < len(credits); i++ {
        totalCredits += credits[i].Amount
    }
    return totalCredits
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
                Rate: 8934000000, Period: 2 }, "ADAUST" },
    }
    totalCredits := sumTotalCredits(credits)
    resTask := eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask := BorrowTask{ 173810000000, []uint64{ 102, 100 }, 4125000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // next testcase (fill all)
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 32455000000, Status: "ACTIVE",
                Rate: 7321000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 128767000000, Status: "ACTIVE",
                Rate: 6663000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 141355000000, Status: "ACTIVE",
                Rate: 8934000000, Period: 2 }, "ADAUST" },
    }
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 302577000000, []uint64{ 102, 100, 101 }, 4125000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // new orderbook
    ob = OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 3471000000 },
            OrderBookEntry{ 11, 3, 20200000000, 3472000000 },
            OrderBookEntry{ 12, 2, 14177000000, 3475000000 },
            OrderBookEntry{ 13, 2, 15320000000, 3480000000 },
            OrderBookEntry{ 14, 2, 27517000000, 3481000000 },
            OrderBookEntry{ 15, 2, 10764000000, 3483000000 },
            OrderBookEntry{ 16, 2, 17520000000, 3485000000 },
        },
    }
    
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 73210000000, Status: "ACTIVE",
                Rate: 6532000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 23120000000, Status: "ACTIVE",
                Rate: 6621100000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 6755000000, Status: "ACTIVE",
                Rate: 5974200000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 3341100000, Status: "ACTIVE",
                Rate: 5975640000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 104, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 2775210000, Status: "ACTIVE",
                Rate: 5784211000, Period: 2 }, "ADAUST" },
    }
    
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 109201310000, []uint64{ 101, 100, 103, 102, 104 }, 3485000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // if lower rate
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 73210000000, Status: "ACTIVE",
                Rate: 4773420000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 23120000000, Status: "ACTIVE",
                Rate: 4556510000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 6755000000, Status: "ACTIVE",
                Rate: 754110000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 3341100000, Status: "ACTIVE",
                Rate: 123410000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 104, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 2775210000, Status: "ACTIVE",
                Rate: 340851000, Period: 2 }, "ADAUST" },
    }
    
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 96330000000, []uint64{ 100, 101 }, 3483000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // skip worse result
    // new orderbook
    ob = OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 2471000000 },
            OrderBookEntry{ 11, 3, 20200000000, 2472000000 },
            OrderBookEntry{ 12, 2, 18548100000, 3475000000 },
            OrderBookEntry{ 13, 2, 19044000000, 5782100000 },
            OrderBookEntry{ 14, 2, 21678000000, 7220300000 },
            OrderBookEntry{ 15, 2, 20114000000, 8221000000 },
            OrderBookEntry{ 16, 2, 12775000000, 8411100000 },
        },
    }
    
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 18742156000, Status: "ACTIVE",
                Rate: 6532000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 12355200000, Status: "ACTIVE",
                Rate: 7834920000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 15676200000, Status: "ACTIVE",
                Rate: 5052610000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 35451100000, Status: "ACTIVE",
                Rate: 5804821000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 104, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 20115600000, Status: "ACTIVE",
                Rate: 4911131000, Period: 2 }, "ADAUST" },
    }
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 82224656000, []uint64{ 101, 100, 103, 102 }, 8221000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 18742156000, Status: "ACTIVE",
                Rate: 6532000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 12355200000, Status: "ACTIVE",
                Rate: 7834920000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour+3*time.Minute),
                UpdateTime: now.Add(-24*time.Hour+3*time.Minute),
                Amount: 15676200000, Status: "ACTIVE",
                Rate: 122110000, Period: 2 }, "ADAUST" }, // do not include!
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 25621200000, Status: "ACTIVE",
                Rate: 8932140000, Period: 2 }, "ADAUST" },
    }
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 56718556000, []uint64{ 103, 101, 100 }, 5782100000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // process normal and expired credits
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 18742156000, Status: "ACTIVE",
                Rate: 6532000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 12355200000, Status: "ACTIVE",
                Rate: 7834920000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-48*time.Hour+3*time.Minute),
                UpdateTime: now.Add(-48*time.Hour+3*time.Minute),
                Amount: 15676200000, Status: "ACTIVE",
                Rate: 122110000, Period: 2 }, "ADAUST" },   // to expire
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 25621200000, Status: "ACTIVE",
                Rate: 8932140000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 104, Currency: "UST", Side: -1,
                CreateTime: now.Add(-48*time.Hour+3*time.Minute),
                UpdateTime: now.Add(-48*time.Hour+3*time.Minute),
                Amount: 9511100000, Status: "ACTIVE",
                Rate: 100110000, Period: 2 }, "ADAUST" },   // to expire
    }
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 81905856000, []uint64{ 103, 101, 100 }, 7220300000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    credits = []Credit{
        Credit{ Loan{ Id: 100, Currency: "UST", Side: -1,
                CreateTime: now.Add(-24*time.Hour),
                UpdateTime: now.Add(-24*time.Hour),
                Amount: 18742156000, Status: "ACTIVE",
                Rate: 6532000000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 101, Currency: "UST", Side: -1,
                CreateTime: now.Add(-23*time.Hour),
                UpdateTime: now.Add(-23*time.Hour),
                Amount: 12355200000, Status: "ACTIVE",
                Rate: 7834920000, Period: 2 }, "BTCUST" },
        Credit{ Loan{ Id: 102, Currency: "UST", Side: -1,
                CreateTime: now.Add(-48*time.Hour+1*time.Minute),
                UpdateTime: now.Add(-48*time.Hour+1*time.Minute),
                Amount: 15676200000, Status: "ACTIVE",
                Rate: 122110000, Period: 2 }, "ADAUST" },   // to expire
        Credit{ Loan{ Id: 103, Currency: "UST", Side: -1,
                CreateTime: now.Add(-22*time.Hour),
                UpdateTime: now.Add(-22*time.Hour),
                Amount: 25621200000, Status: "ACTIVE",
                Rate: 8932140000, Period: 2 }, "ADAUST" },
        Credit{ Loan{ Id: 104, Currency: "UST", Side: -1,
                CreateTime: now.Add(-48*time.Hour+7*time.Minute),
                UpdateTime: now.Add(-48*time.Hour+7*time.Minute),
                Amount: 9511100000, Status: "ACTIVE",
                Rate: 100110000, Period: 2 }, "ADAUST" },   // to expire
    }
    totalCredits = sumTotalCredits(credits)
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 81905856000, []uint64{ 103, 101, 100 }, 7220300000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    oldCredits := credits
    oldOb := ob
    // if orderbook is too short
    ob = OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 2471000000 },
            OrderBookEntry{ 11, 3, 20200000000, 2472000000 },
            OrderBookEntry{ 12, 2, 18548100000, 3475000000 },
            OrderBookEntry{ 13, 2, 19044000000, 5782100000 },
        },
    }
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 72394756000, []uint64{ 103, 101, 100 }, 5782100000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    // if orderbook is too short 2
    ob = OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 2471000000 },
            OrderBookEntry{ 11, 3, 20200000000, 2472000000 },
            OrderBookEntry{ 12, 2, 18548100000, 3475000000 },
        },
    }
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 37976400000, []uint64{ 103, 101 }, 3475000000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    
    // include total borrow rests
    ob = oldOb
    credits = oldCredits
    totalCredits = sumTotalCredits(credits) + 221344000
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 82127200000, []uint64{ 103, 101, 100 }, 7220300000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
    // and if orderbook too short
    ob = OrderBook{
        Bid: []OrderBookEntry{
            OrderBookEntry{ 0, 2, 16000000000, 6611000000 },
            OrderBookEntry{ 1, 2, 16000000000, 5221000000 },
        },
        Ask: []OrderBookEntry{
            OrderBookEntry{ 10, 2, 16000000000, 2471000000 },
            OrderBookEntry{ 11, 3, 20200000000, 2472000000 },
            OrderBookEntry{ 12, 2, 18548100000, 3475000000 },
            OrderBookEntry{ 13, 2, 19044000000, 5782100000 },
            OrderBookEntry{ 14, 2, 8330000000, 7220300000 },
        },
    }
    resTask = eng.prepareBorrowTask(&ob, credits, totalCredits, now)
    expTask = BorrowTask{ 82122100000, []uint64{ 103, 101, 100 }, 7220300000 }
    if !equalBorrowTask(&expTask, &resTask) {
        t.Errorf("BorrowTask mismatch: %v!=%v", expTask, resTask)
    }
}
