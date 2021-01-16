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
    "time"
    "github.com/matszpk/godec128"
)

type Config struct {
    Currency string
    // total borrowed assets - can be zero, then get from used credits
    TotalBorrowed godec128.UDec128
    // when same Bitfinex fetch loans for positions
    AutoLoanFetchPeriod time.Duration
    // start time before expiration
    StartBeforeExpire time.Duration
    // max acceptable rate for typical times
    TypicalMaxRate godec128.UDec128
    // max acceptable rate for unlucky times
    UnluckyMaxRate godec128.UDec128
    // preferrable rate
    TypicalPreferredRate godec128.UDec128
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
