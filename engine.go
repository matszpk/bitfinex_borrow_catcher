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
    configStrStartTimeBeforeExpiration = []byte("startTimeBeforeExpiration")
    configStrSettlementCostFactor = []byte("settlementCostFactor")
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
    StartTimeBeforeExpiration time.Duration
    // settlement cost factor
    SettlementCostFactor float64
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
        if ((mask & 16) == 0 && bytes.Equal(key, configStrStartTimeBeforeExpiration)) {
            config.StartTimeBeforeExpiration = FastjsonGetDuration(vx)
            mask |= 16
        }
        if ((mask & 32) == 0 && bytes.Equal(key, configStrSettlementCostFactor)) {
            config.SettlementCostFactor = FastjsonGetFloat64(vx)
            mask |= 32
        }
        if ((mask & 64) == 0 && bytes.Equal(key, configStrMinRateDifference)) {
            config.MinRateDifference = FastjsonGetFloat64(vx)
            mask |= 64
        }
        if ((mask & 128) == 0 && bytes.Equal(key, configStrMinOrderAmount)) {
            config.MinOrderAmount = FastjsonGetUDec64(vx, 12)
            mask |= 128
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

/*func (eng *Engine) PrepareBorrowTask(ob *OrderBook) {
    poss := eng.bpriv.GetPositions()
    posCosts := make(map[string]godec64.UDec64)
}*/

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
