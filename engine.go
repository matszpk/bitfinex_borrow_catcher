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
    "io/ioutil"
    "os"
    "sort"
    "sync"
    "time"
    "github.com/valyala/fastjson"
    "github.com/matszpk/godec64"
)

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
}

/* Engine stats */

type LastStatsPeriod struct {
    t time.Duration
    Min godec64.UDec64
    Avg godec64.UDec64
    Max godec64.UDec64
}

type EngineStats struct {
    candle []Candle
    min10Stats LastStatsPeriod
    min30Stats LastStatsPeriod
    hourStats LastStatsPeriod
}

/* Engine stuff */

type Engine struct {
    stopCh chan struct{}
    baseCurrMarkets map[string]bool
    quoteCurrMarkets map[string]bool
    config *Config
    df *DataFetcher
    bpriv *BitfinexPrivate
    stats EngineStats
    mutex sync.Mutex
}

func NewEngine(config *Config, df *DataFetcher, bpriv *BitfinexPrivate) *Engine {
    return &Engine{ stopCh: make(chan struct{}),
                baseCurrMarkets: make(map[string]bool),
                quoteCurrMarkets: make(map[string]bool),
                config: config, df: df, bpriv: bpriv }
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
    eng.df.SetLastTradeHandler(eng.checkLastTrade)
    go eng.mainRoutine()
}

func (eng *Engine) Stop() {
    eng.stopCh <- struct{}{}
    eng.df.SetOrderBookHandler(nil)
    eng.df.SetLastTradeHandler(nil)
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
    Logger.Info("ccc:", normCredits)
    
    obFill := func(csAmount godec64.UDec64) (float64, bool) {
        var obAmountRate float64 = 0
        for ; obi < oblen && csAmount >= ob.Ask[obi].Amount - obFilled ; obi++ {
            Logger.Info("dddy:", ob.Ask[obi])
            obAmount := (ob.Ask[obi].Amount - obFilled).ToFloat64(8)
            obAmountRate += obAmount * ob.Ask[obi].Rate.ToFloat64(12)
            obTotalAmount += obAmount
            csAmount -= ob.Ask[obi].Amount - obFilled
            obFilled = 0
        }
        if obi == oblen && csAmount != 0 {
            return obAmountRate, false
        }
        if obi != oblen && csAmount < ob.Ask[obi].Amount - obFilled {
            obAmount := csAmount.ToFloat64(8)
            obAmountRate += obAmount * ob.Ask[obi].Rate.ToFloat64(12)
            obTotalAmount += obAmount
            obFilled += csAmount
        }
        return obAmountRate, true
    }
    
    // find balance between orderbook average rate and credits average rate.
    // find orderbook average rate starting from lowest orders to highest orders.
    // find credits average rate starting from highest to lowest rate.
    for csi := len(normCredits)-1 ;csi >= 0; csi-- {
        csAmount := normCredits[csi].Amount
        Logger.Info("cccx:", normCredits[csi])
        // map credit to orderbook offers.
        csEntryAmount := csAmount.ToFloat64(8)
        csAmountRate := csEntryAmount * normCredits[csi].Rate.ToFloat64(12)
        
        obAmountRate, left := obFill(csAmount)
        if !left { break }
        Logger.Info("cccy:", csAmountRate, obAmountRate)
        
        // check whether result is not worse than in highest credit loan
        var hcsAmountRate float64 = 0
        hcsi := len(normCredits)-1
        for ; hcsi >= 0 && csAmount >= normCredits[hcsi].Amount; hcsi-- {
            hcsAmount := (normCredits[hcsi].Amount).ToFloat64(8)
            hcsAmountRate += hcsAmount * normCredits[hcsi].Rate.ToFloat64(12)
        }
        if hcsi >= 0 && csAmount < normCredits[hcsi].Amount {
            hcsAmount := csAmount.ToFloat64(8)
            hcsAmountRate += hcsAmount * normCredits[hcsi].Rate.ToFloat64(12)
        }
        
        csAmount = normCredits[csi].Amount
        
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
        if _, left := obFill(toExpireCredits[i].Amount); !left { break }
        // if really expire in this loan fetch period,
        // do not add to list of loans to close.
        task.TotalBorrow += toExpireCredits[i].Amount
    }
    
    // fill rest of not borrowed from total borrow
    if totalBorrow > totalCredits {
        obFill(totalBorrow - totalCredits)
    }
    
    return task
}

func (eng *Engine) checkOrderBook(ob *OrderBook) {
    if len(ob.Ask)==0 {
        return // no offers
    }
}

func (eng *Engine) checkLastTrade(tr *Trade) {
}

func (eng *Engine) mainRoutine() {
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
