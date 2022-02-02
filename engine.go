/*
 * engine.go - main engine module
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
    "bytes"
    "crypto/rand"
    "io"
    "io/ioutil"
    "os"
    "sort"
    "sync"
    "time"
    "github.com/valyala/fastjson"
    "github.com/matszpk/godec64"
)

func getRandom(n int64) int64 {
    var b [8]byte
    if _, err := io.ReadFull(rand.Reader, b[:]); err == nil {
        v := int64(b[0]) + (int64(b[1])<<8) + (int64(b[2])<<16) + (int64(b[3])<<24) +
            (int64(b[4])<<32) + (int64(b[5])<<40) + (int64(b[6])<<48) +
            (int64(b[7]&0x7f)<<56)
        return v % n
    } else {
        ErrorPanic("Can't get random number", err)
    }
    return 0
}

/* Config stuff */

var (
    configStrCurrency = []byte("currency")
    configStrAutoLoanFetchPeriod = []byte("autoLoanFetchPeriod")
    configStrAutoLoanFetchShift = []byte("autoLoanFetchShift")
    configStrAutoLoanFetchEndShift = []byte("autoLoanFetchEndShift")
    configStrMinRateDifference = []byte("minRateDifference")
    configStrMinOrderAmount = []byte("minOrderAmount")
)

type Config struct {
    Currency string
    // when same Bitfinex fetch loans for positions in second
    AutoLoanFetchPeriod time.Duration
    // when same bitfinex fetch loans for positions - shift in second
    AutoLoanFetchShift time.Duration
    AutoLoanFetchEndShift time.Duration
    MinRateDifference float64
    MinOrderAmount godec64.UDec64
}

func configFromJson(v *fastjson.Value, config *Config) {
    *config = Config{}
    mask := 0
    obj := FastjsonGetObjectRequired(v)
    obj.Visit(func(key []byte, vx *fastjson.Value) {
        if ((mask & 1) == 0 && bytes.Equal(key, configStrCurrency)) {
            config.Currency = FastjsonGetString(vx)
            mask |= 1
        }
        if ((mask & 2) == 0 && bytes.Equal(key, configStrAutoLoanFetchPeriod)) {
            config.AutoLoanFetchPeriod = FastjsonGetDuration(vx)
            mask |= 2
        }
        if ((mask & 4) == 0 && bytes.Equal(key, configStrAutoLoanFetchShift)) {
            config.AutoLoanFetchShift = FastjsonGetDuration(vx)
            mask |= 4
        }
        if ((mask & 8) == 0 && bytes.Equal(key, configStrAutoLoanFetchEndShift)) {
            config.AutoLoanFetchEndShift = FastjsonGetDuration(vx)
            mask |= 8
        }
        if ((mask & 16) == 0 && bytes.Equal(key, configStrMinRateDifference)) {
            config.MinRateDifference = FastjsonGetFloat64(vx)
            mask |= 16
        }
        if ((mask & 32) == 0 && bytes.Equal(key, configStrMinOrderAmount)) {
            config.MinOrderAmount = FastjsonGetUDec64(vx, 12)
            mask |= 32
        }
    })
}

func (config *Config) Load(filename string) {
    f, err := os.Open(filename)
    if err!=nil {
        ErrorPanic("Can't open config file", err)
    }
    defer f.Close()
    b, err := ioutil.ReadAll(f)
    if err!=nil {
        ErrorPanic("Can't read config file", err)
    }
    jp := JsonParserPool.Get()
    defer JsonParserPool.Put(jp)
    if v, err := jp.ParseBytes(b); err==nil {
        configFromJson(v, config)
    } else {
        ErrorPanic("Can't parse config file", err)
    }
}

type BorrowTask struct {
    TotalBorrow godec64.UDec64
    LoanIdsToClose []uint64
    Rate godec64.UDec64
}

func (bt *BorrowTask) Join(next *BorrowTask) {
    bt.TotalBorrow += next.TotalBorrow
    bt.LoanIdsToClose = append(bt.LoanIdsToClose, next.LoanIdsToClose...)
}

/* Engine stuff */

type Engine struct {
    stopCh chan struct{}
    baseCurrMarkets map[string]bool
    quoteCurrMarkets map[string]bool
    config *Config
    df *DataFetcher
    bpriv *BitfinexPrivate
    obCh chan *OrderBook
    taskMutex sync.Mutex
}

func NewEngine(config *Config, df *DataFetcher, bpriv *BitfinexPrivate) *Engine {
    return &Engine{ stopCh: make(chan struct{}),
                baseCurrMarkets: make(map[string]bool),
                quoteCurrMarkets: make(map[string]bool),
                config: config, df: df, bpriv: bpriv,
                obCh: make(chan *OrderBook) }
}

func (eng *Engine) PrepareMarkets() {
    bp := eng.df.GetPublic()
    markets := bp.GetMarkets()
    for _, m := range markets {
        if  eng.config.Currency == m.BaseCurrency {
            eng.baseCurrMarkets[eng.config.Currency] = true
        } else if  eng.config.Currency == m.QuoteCurrency {
            eng.quoteCurrMarkets[eng.config.Currency] = true
        }
    }
}

func (eng *Engine) Start() {
    eng.df.SetOrderBookHandler(eng.checkOrderBook)
    go eng.mainRoutine()
}

func (eng *Engine) Stop() {
    eng.stopCh <- struct{}{}
    eng.df.SetOrderBookHandler(nil)
    close(eng.obCh)
}

type CreditsSort []Credit

func (cs CreditsSort) Len() int {
    return len(cs)
}

func (cs CreditsSort) Less(i, j int) bool {
    return cs[i].Rate < cs[j].Rate
}

func (cs CreditsSort) Swap(i, j int) {
    cs[i], cs[j] = cs[j], cs[i]
}

func (eng *Engine) calculateTotalBorrow(poss []Position, bals []Balance) godec64.UDec64 {
    var totalBal godec64.UDec64 = 0
    for i := 0; i < len(bals); i++ {
        if bals[i].Currency == eng.config.Currency {
            totalBal = bals[i].Total
            break
        }
    }
    
    var posTotalVal godec64.UDec64 = 0
    for i := 0; i < len(poss); i++ {
        pos := &poss[i]
        if pos.Long {
            if _, ok :=  eng.quoteCurrMarkets[pos.Market]; !ok {
                continue // if not this market
            }
            posTotalVal += poss[i].Amount.Mul(poss[i].BasePrice, 8, true)
        } else { // short
            if _, ok :=  eng.baseCurrMarkets[pos.Market]; !ok {
                continue // if not this market
            }
            posTotalVal += poss[i].Amount
        }
    }
    if posTotalVal > totalBal {
        return posTotalVal - totalBal
    } else { return 0 }
}

func (eng *Engine) prepareBorrowTask(ob *OrderBook, credits []Credit,
                            totalBorrow godec64.UDec64, now time.Time) BorrowTask {
    var totalCredits godec64.UDec64
    for i := 0; i < len(credits); i++ {
        totalCredits += credits[i].Amount
    }
    
    oblen := len(ob.Ask)
    
    var task BorrowTask
    if oblen == 0 { return task }
    if len(credits) == 0 { return task }
    
    var normCredits, toExpireCredits []Credit
    for i := 0; i < len(credits); i++ {
        credit := &credits[i]
        expireTime := credit.CreateTime.Add(24*time.Hour*time.Duration(credit.Period))
        afterAutoLoanTime := now.Truncate(eng.config.AutoLoanFetchPeriod).
                Add(eng.config.AutoLoanFetchShift)
        if afterAutoLoanTime.Before(now) {
            // if still before now
            afterAutoLoanTime = afterAutoLoanTime.Add(eng.config.AutoLoanFetchPeriod)
        }
        if !afterAutoLoanTime.After(expireTime) { // if normal
            normCredits = append(normCredits, *credit)
        } else {
            toExpireCredits = append(toExpireCredits, *credit)
        }
    }
    
    sort.Sort(CreditsSort(normCredits))
    var obSumAmountRate float64 = 0
    var csSumAmountRate float64 = 0
    var obTotalAmount float64 = 0
    var csTotalAmount float64 = 0
    obi := 0
    var obFilled godec64.UDec64 = 0
    
    obFill := func(csAmount godec64.UDec64) (godec64.UDec64, float64, bool) {
        var obAmountRate float64 = 0
        for ; obi < oblen && csAmount >= ob.Ask[obi].Amount - obFilled ; obi++ {
            obAmount := (ob.Ask[obi].Amount - obFilled).ToFloat64(8)
            obAmountRate += obAmount * ob.Ask[obi].Rate.ToFloat64(12)
            obTotalAmount += obAmount
            csAmount -= ob.Ask[obi].Amount - obFilled
            obFilled = 0
            task.Rate = ob.Ask[obi].Rate
        }
        if obi == oblen && csAmount != 0 {
            return csAmount, obAmountRate, false
        }
        if obi != oblen && csAmount != 0 && csAmount < ob.Ask[obi].Amount - obFilled {
            obAmount := csAmount.ToFloat64(8)
            obAmountRate += obAmount * ob.Ask[obi].Rate.ToFloat64(12)
            obTotalAmount += obAmount
            obFilled += csAmount
            csAmount = 0
            task.Rate = ob.Ask[obi].Rate
        }
        return csAmount, obAmountRate, true
    }
    
    // find balance between orderbook average rate and credits average rate.
    // find orderbook average rate starting from lowest orders to highest orders.
    // find credits average rate starting from highest to lowest rate.
    for csi := len(normCredits)-1 ;csi >= 0; csi-- {
        csAmount := normCredits[csi].Amount
        // map credit to orderbook offers.
        csEntryAmount := csAmount.ToFloat64(8)
        csAmountRate := csEntryAmount * normCredits[csi].Rate.ToFloat64(12)
        
        _, obAmountRate, left := obFill(csAmount)
        if !left { break }
        
        // check whether current rate is not lower than best rate in orderbook
        csAmountLeft := csAmount
        lowestObi := 0
        var lowObAmountRate float64
        for ; lowestObi < oblen && csAmountLeft >= ob.Ask[lowestObi].Amount; lowestObi++ {
            obAmount := ob.Ask[lowestObi].Amount.ToFloat64(8)
            lowObAmountRate += obAmount * ob.Ask[lowestObi].Rate.ToFloat64(12)
            csAmountLeft -= ob.Ask[lowestObi].Amount
        }
        if lowestObi != oblen && csAmountLeft < ob.Ask[lowestObi].Amount {
            obAmount := csAmountLeft.ToFloat64(8)
            lowObAmountRate += obAmount * ob.Ask[lowestObi].Rate.ToFloat64(12)
            csAmountLeft = 0
        }
        // if calculated
        if csAmountLeft == 0 {
            if csAmountRate < lowObAmountRate {
                break  // if credit rate is lower than lowest lowObAmountRate
            }
        }
        
        // check whether result is not worse than in highest credit loan
        var hcsAmountRate float64 = 0
        hcsi := len(normCredits)-1
        csAmountLeft = csAmount
        for ; hcsi >= 0 && csAmountLeft >= normCredits[hcsi].Amount; hcsi-- {
            hcsAmount := (normCredits[hcsi].Amount).ToFloat64(8)
            hcsAmountRate += hcsAmount * normCredits[hcsi].Rate.ToFloat64(12)
            csAmountLeft -= normCredits[hcsi].Amount
        }
        if hcsi >= 0 && csAmountLeft < normCredits[hcsi].Amount {
            hcsAmount := csAmountLeft.ToFloat64(8)
            hcsAmountRate += hcsAmount * normCredits[hcsi].Rate.ToFloat64(12)
        }
        
        if hcsAmountRate < obAmountRate { break }
        
        obSumAmountRate += obAmountRate
        csSumAmountRate += csAmountRate
        csTotalAmount += csEntryAmount
        if obSumAmountRate / obTotalAmount <= (csSumAmountRate / csTotalAmount) *
                (1.0 - eng.config.MinRateDifference) {
            task.LoanIdsToClose = append(task.LoanIdsToClose, normCredits[csi].Id)
            task.TotalBorrow += csAmount
        } else { break }
    }
    
    // to expire credits
    for i := 0; i < len(toExpireCredits); i++ {
        // map credit to orderbook offers.
        if _, _, left := obFill(toExpireCredits[i].Amount); !left { break }
        // if really expire in this loan fetch period,
        // do not add to list of loans to close.
        task.TotalBorrow += toExpireCredits[i].Amount
    }
    
    // fill rest of not borrowed from total borrow
    if totalBorrow > totalCredits {
        rest := totalBorrow - totalCredits
        amountLeft, _, _:= obFill(rest)
        task.TotalBorrow += rest - amountLeft
    }
    return task
}

func (eng *Engine) checkOrderBook(ob *OrderBook) {
    eng.obCh <- ob
}

func (eng *Engine) doBorrowTask(bt *BorrowTask) bool {
    var opr OpResult
    eng.bpriv.SubmitBidOrder(eng.config.Currency, bt.TotalBorrow,
                            bt.Rate.Mul(1100000000000, 12, true), 2, &opr)
    if !opr.Success {
        Logger.Error("doBorrowTask SubmitBidOrder failed:", opr.Message)
        return false
    }
    time.Sleep(2*time.Second)
    // check whether is fully filled
    orders := eng.bpriv.GetActiveOrders(eng.config.Currency)
    oidx := 0
    for ; oidx < len(orders); oidx++ {
        if opr.Order.Id == orders[oidx].Id { break }
    }
    if oidx != len(orders) {  // found and then not fully filled
        time.Sleep(10*time.Second) // for some time
        // and cancel
        oid := opr.Order.Id
        eng.bpriv.CancelOrder(oid, &opr)
    } // if fully filled
    
    // now close fundings
    for _, loanId := range bt.LoanIdsToClose {
        var op2r Op2Result
        eng.bpriv.CloseFunding(loanId, &op2r)
        if !op2r.Success {
            Logger.Error("doBorrowTask CloseFunding failed:", op2r.Message)
            return false
        }
    }
    return true
}

func (eng *Engine) doCloseUnusedFundings() bool {
    loans := eng.bpriv.GetLoans(eng.config.Currency)
    for i := 0; i < len(loans); i++ {
        var op2r Op2Result
        eng.bpriv.CloseFunding(loans[i].Id, &op2r)
        if !op2r.Success {
            Logger.Error("doCloseUnusedFundings CloseFunding failed:", op2r.Message)
            return false
        }
    }
    return true
}

func (eng *Engine) doCloseUnusedFundingsSafe() bool {
    defer func() {
        if x := recover(); x!=nil {
            Logger.Error("Panic in doCloseUnusedFundings:", x)
        }
    }()
    return eng.doCloseUnusedFundingsSafe()
}

func (eng *Engine) makeBorrowTask(t time.Time) {
    eng.taskMutex.Lock()
    defer eng.taskMutex.Unlock()
    credits := eng.bpriv.GetCredits(eng.config.Currency)
    bals := eng.bpriv.GetMarginBalances()
    poss := eng.bpriv.GetPositions()
    totalBorrow := eng.calculateTotalBorrow(poss, bals)
    var ob OrderBook
    eng.df.GetPublic().GetMaxOrderBook(eng.config.Currency, &ob)
    bt := eng.prepareBorrowTask(&ob, credits, totalBorrow, t)
    eng.doBorrowTask(&bt)
}

func (eng *Engine) makeBorrowTaskSafe(t time.Time) {
    defer func() {
        if x := recover(); x!=nil {
            Logger.Error("Panic in makeBorrowTask:", x)
        }
    }()
    eng.makeBorrowTask(t)
}

// return true if auto loan period passed, otherwise if engine stopped.
func (eng *Engine) handleAutoLoanPeriod(alPeriodTime time.Time) bool {
    alDur := eng.config.AutoLoanFetchEndShift - eng.config.AutoLoanFetchShift
    if alDur < 0 { alDur =- alDur }
    alEndTimer := time.NewTimer(alPeriodTime.Add(alDur).Sub(time.Now()))
    defer alEndTimer.Stop()
    taskTimer := time.NewTimer(alPeriodTime.Add(alDur -
            (time.Duration(getRandom(60000))+100)*time.Millisecond).Sub(time.Now()))
    defer taskTimer.Stop()
    
    eng.doCloseUnusedFundingsSafe()
    
    btDone := false
    var lastOb *OrderBook
    
    for {
        select {
            case ob := <-eng.obCh: {
                if len(lastOb.Ask) != 0 && len(ob.Ask) != 0 {
                    if lastOb.Ask[0].Rate < ob.Ask[0].Rate {
                        // some eat orderbook, initialize makeBorrowTask
                        go eng.makeBorrowTaskSafe(time.Now())
                    }
                }
                lastOb = ob
            }
            case t := <-taskTimer.C:
                if !btDone {
                    go eng.makeBorrowTaskSafe(t)
                    btDone = true
                }
            case <-alEndTimer.C:
                return true
            case <-eng.stopCh:
                return false
        }
    }
    return true
}

func (eng *Engine) mainRoutine() {
    now := time.Now()
    alPeriodTime := now.Truncate(eng.config.AutoLoanFetchPeriod).
                Add(eng.config.AutoLoanFetchShift)
    if alPeriodTime.After(now) { // go to back
        alPeriodTime = alPeriodTime.Add(-eng.config.AutoLoanFetchPeriod)
    }
    
    // main loop
    for {
        if !eng.handleAutoLoanPeriod(alPeriodTime) { break }
        alPeriodTime = alPeriodTime.Add(eng.config.AutoLoanFetchPeriod)
    }
    /*ticker := time.NewTicker(engCheckStatusPeriod)
    defer ticker.Stop()
    
    stopped := false
    for !stopped {
        select {
            case <- ticker.C:
            case <- eng.stopCh:
                stopped = true
        }
    }*/
}
