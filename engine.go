/*
 * engine.go - data fetching module
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
    "github.com/matszpk/godec64"
    "github.com/valyala/fastjson"
)

const engCheckStatusPeriod = time.Second*30

/* Config stuff */

var (
    configStrCurrency = []byte("currency")
    configStrTotalBorrowed = []byte("totalBorrowed")
    configStrAutoLoanFetchPeriod = []byte("auotLoanFetchPeriod")
    configStrStartBeforeExpire = []byte("startBeforeExpire")
    configStrTypicalMaxRate = []byte("typicalMaxRate")
    configStrUnluckyMaxRate = []byte("unluckyMaxRate")
    configStrPreferredRate = []byte("preferredRate")
    configStrMaxFundingPeriod = []byte("maxFundingPeriod")
)

type Config struct {
    Currency string
    // total borrowed assets - can be zero, then get from used credits
    TotalBorrowed godec64.UDec64
    // when same Bitfinex fetch loans for positions in second
    AutoLoanFetchPeriod time.Duration
    // start time before expiration
    StartBeforeExpire time.Duration
    // max acceptable rate for typical times
    TypicalMaxRate godec64.UDec64
    // max acceptable rate for unlucky times
    UnluckyMaxRate godec64.UDec64
    // preferrable rate
    PreferredRate godec64.UDec64
    MaxFundingPeriod uint32
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
        if ((mask & 2) == 0 && bytes.Equal(key, configStrTotalBorrowed)) {
            config.TotalBorrowed = FastjsonGetUDec64(vx, 8)
            mask |= 2
        }
        if ((mask & 4) == 0 && bytes.Equal(key, configStrAutoLoanFetchPeriod)) {
            config.AutoLoanFetchPeriod = FastjsonGetDuration(vx)
            mask |= 4
        }
        if ((mask & 8) == 0 && bytes.Equal(key, configStrStartBeforeExpire)) {
            config.StartBeforeExpire = FastjsonGetDuration(vx)
            mask |= 8
        }
        if ((mask & 16) == 0 && bytes.Equal(key, configStrTypicalMaxRate)) {
            config.TypicalMaxRate = FastjsonGetUDec64(vx, 12)
            mask |= 16
        }
        if ((mask & 32) == 0 && bytes.Equal(key, configStrUnluckyMaxRate)) {
            config.UnluckyMaxRate = FastjsonGetUDec64(vx, 12)
            mask |= 32
        }
        if ((mask & 64) == 0 && bytes.Equal(key, configStrPreferredRate)) {
            config.PreferredRate = FastjsonGetUDec64(vx, 12)
            mask |= 64
        }
        if ((mask & 128) == 0 && bytes.Equal(key, configStrMaxFundingPeriod)) {
            config.MaxFundingPeriod = FastjsonGetUInt32(vx)
            mask |= 128
        }
    })
    // fix rate from percent to (0-1)
    config.TypicalMaxRate = config.TypicalMaxRate / 100
    config.UnluckyMaxRate = config.UnluckyMaxRate/ 100
    config.PreferredRate = config.PreferredRate / 100
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

/* borrow queue */

type BorrowQueueElem struct {
    ExpireTime time.Time
    ToBorrow godec64.UDec64
}

type BorrowQueue struct {
    startPos int
    length int
    array []BorrowQueueElem
}

func (bq *BorrowQueue) Value(i int) BorrowQueueElem {
    if i >= bq.length {
        panic("Index overflow")
    }
    return bq.array[(bq.startPos + i) % len(bq.array)]
}

// get minimal length for required amount and total to borrow
func (bq *BorrowQueue) MinLengthToOffer(
            required godec64.UDec64) (length int, total godec64.UDec64) {
    i := 0
    total = 0
    k := bq.startPos
    arrLen := len(bq.array)
    for i=0; i < bq.length; i++ {
        if total > required {
            break
        }
        e := bq.array[k]
        k++
        if k >= arrLen { k = 0 }
        total += e.ToBorrow
    }
    length = i
    return
}

func (bq *BorrowQueue) newArray() {
    // create new longer array
    alen := len(bq.array)
    newArray := make([]BorrowQueueElem, (bq.length+1)*2)
    k := bq.startPos
    for i := 0; i < bq.length; i++ {
        newArray[i] = bq.array[k]
        k++
        if k >= alen { k = 0 }
    }
    bq.array = newArray
    bq.startPos = 0
}

func (bq *BorrowQueue) Push(e BorrowQueueElem) {
    alen := len(bq.array)
    if bq.length >= alen {
        bq.newArray()
    }
    bq.array[(bq.startPos + bq.length) % alen] = e
    bq.length++
}

func (bq *BorrowQueue) Pop() BorrowQueueElem {
    if bq.length == 0 {
        panic("No elements in queue")
    }
    elem := bq.array[bq.startPos]
    bq.startPos++
    alen := len(bq.array)
    if bq.startPos >= alen {
        bq.startPos = 0
    }
    bq.length--
    if bq.length*4 < alen {
        // shrink array if too many free cells
        bq.newArray()
    }
    return elem
}

/* Engine stuff */

type Engine struct {
    stopCh chan struct{}
    config *Config
    df *DataFetcher
    bpriv *BitfinexPrivate
    autoFetchShiftTimeSet bool
    autoFetchShiftTime time.Duration
    stats EngineStats
    mutex sync.Mutex
    borrowQueue BorrowQueue
}

func NewEngine(config *Config, df *DataFetcher, bpriv *BitfinexPrivate) *Engine {
    return &Engine{ stopCh: make(chan struct{}), config: config,
                df: df, bpriv: bpriv }
}

func (eng *Engine) Start() {
    eng.df.SetOrderBookHandler(eng.checkOrderBook)
    go eng.mainRoutine()
}

func (eng *Engine) Stop() {
    eng.stopCh <- struct{}{}
    eng.df.SetOrderBookHandler(nil)
}

func (eng *Engine) checkOrderBook(ob *OrderBook) {
    if len(ob.Ask)==0 {
        return // no offers
    }
}

func (eng *Engine) pushPendingBorrows() {
    eng.mutex.Lock()
    defer eng.mutex.Unlock()
    //credits := eng.bpriv.GetCredits(eng.DataFetch.GetCurrency())
}

func (eng *Engine) mainRoutine() {
    ticker := time.NewTicker(engCheckStatusPeriod)
    defer ticker.Stop()
    
    stopped := false
    for !stopped {
        select {
            case <- ticker.C:
            case <- eng.stopCh:
                stopped = true
        }
    }
}
