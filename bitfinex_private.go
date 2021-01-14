/*
 * bitfinex_private.go - Bitfinex Private client
 *
 * bitfinex_funding_catcher - Automatic funding catcher for open positions in
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
    "crypto/hmac"
    "crypto/sha512"
    "encoding/hex"
    "strconv"
    "time"
    "github.com/valyala/fasthttp"
    "github.com/valyala/fastjson"
)

var (
    bitfinexStrNonce = []byte("bfx-nonce")
    bitfinexStrApiKey = []byte("bfx-apkikey")
    bitfinexStrSignature = []byte("bfx-signature")
    bitfinexStrApiPrefix = []byte("/api/")
)

type BitfinexPrivate struct {
    httpClient fasthttp.HostClient
    apiKey, apiSecret []byte
}

func (drv *BitfinexPrivate) handleHttpPostJson(rh *RequestHandle,
                host, uri []byte, body *fastjson.Value) (*fastjson.Value, int) {
    nonceB := strconv.AppendInt(nil ,time.Now().UnixNano(), 10)
    // generate signature
    bodyStr, err := body.StringBytes()
    if err!=nil {
        ErrorPanic("Can't get body string for HTTP POST request", err)
    }
    sig := make([]byte, 0, 200)
    sig = append(sig, bitfinexStrApiPrefix...)
    sig = append(sig, uri...)
    sig = append(sig, nonceB...)
    sig = append(sig, bodyStr...)
    
    sumGen := hmac.New(sha512.New384, drv.apiSecret)
    sumGen.Write(sig)
    sum := sumGen.Sum(nil)
    sumHex := make([]byte, len(sum)*2)
    hex.Encode(sumHex, sum)
    
    headers := [][]byte{
        bitfinexStrNonce, nonceB, bitfinexStrApiKey, drv.apiKey,
        bitfinexStrSignature, sumHex }
    
    return rh.HandleHttpPostJson(drv.httpClient, host, uri, bodyStr, headers)
}
