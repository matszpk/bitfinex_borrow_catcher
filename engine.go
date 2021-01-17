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
    "time"
    "github.com/matszpk/godec128"
    "github.com/valyala/fastjson"
)

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
    TotalBorrowed godec128.UDec128
    // when same Bitfinex fetch loans for positions in second
    AutoLoanFetchPeriod time.Duration
    // start time before expiration
    StartBeforeExpire time.Duration
    // max acceptable rate for typical times
    TypicalMaxRate godec128.UDec128
    // max acceptable rate for unlucky times
    UnluckyMaxRate godec128.UDec128
    // preferrable rate
    PreferredRate godec128.UDec128
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
            config.TotalBorrowed = FastjsonGetUDec128(vx, 8)
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
            config.TypicalMaxRate = FastjsonGetUDec128(vx, 12)
            mask |= 16
        }
        if ((mask & 32) == 0 && bytes.Equal(key, configStrUnluckyMaxRate)) {
            config.UnluckyMaxRate = FastjsonGetUDec128(vx, 12)
            mask |= 32
        }
        if ((mask & 64) == 0 && bytes.Equal(key, configStrPreferredRate)) {
            config.PreferredRate = FastjsonGetUDec128(vx, 12)
            mask |= 64
        }
        if ((mask & 128) == 0 && bytes.Equal(key, configStrMaxFundingPeriod)) {
            config.MaxFundingPeriod = FastjsonGetUInt32(vx)
            mask |= 128
        }
    })
}

func LoadConfig(filename string, config *Config) {
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

type Engine struct {
    config *Config
    df *DataFetcher
}

func NewEngine(config *Config, df *DataFetcher) *Engine {
    return &Engine{ config: config, df: df }
}

func (eng *Engine) Start() {
}

func (eng *Engine) Stop() {
}
