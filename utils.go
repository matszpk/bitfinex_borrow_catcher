/*
 * utils.go - utilities
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
    "fmt"
    "os"
    "github.com/kataras/golog"
)

// strings
var (
    utilsStrFatalError = []byte("Fatal error while handling panic and exit\n")
)

var Logger *golog.Logger = golog.New()

func init() {
    Logger.SetTimeFormat("2006-01-02 15:04:05")
}

func RecoverPanic(name string) {
    if x := recover(); x!=nil {
        Logger.Error("Panic in ", name , ": ", x, "\n")
    }
}

func RecoverPanicLogger(child *golog.Logger, name string) {
    if x := recover(); x!=nil {
        child.Error("Panic in ", name , ": ", x, "\n")
    }
}

func FatalRecoverPanicAndExit() {
    if x := recover(); x!=nil {
        os.Stderr.Write(utilsStrFatalError)
        os.Exit(1)
    }
}

func RecoverPanicAndExit(name string) {
    defer FatalRecoverPanicAndExit()
    if x := recover(); x!=nil {
        Logger.Error("Panic in ", name , ": ", x)
        os.Exit(1)
    }
}

func ErrorPanic(msg string, err error) {
    panic(fmt.Sprint(msg, ": ", err))
}
