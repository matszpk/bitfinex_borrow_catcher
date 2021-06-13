/*
 * httpclient.go - HTTP client
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
    "fmt"
    "math"
    "time"
    "github.com/matszpk/godec128"
    "github.com/valyala/fasthttp"
    "github.com/valyala/fastjson"
)

func HttpPanic(msg string, statusCode int) {
    panic(fmt.Sprint(msg, ": status code: ", fasthttp.StatusMessage(statusCode),
                     " (", statusCode, ")"))
}

var jsonContentType []byte = []byte("application/json")

// check whether is content-type application/json ?
func CheckJsonContentType(respContentType []byte) bool {
    rlen := len(respContentType)
    if rlen<16 || !bytes.Equal(respContentType[:16], jsonContentType) {
        return false
    }
    if rlen==16 { return true }
    i := 16
    for ; i<rlen && respContentType[i]==' '; i++ { } // skip spaces
    // no semicolon
    if i>=rlen || respContentType[i]!=';' { return false }
    return true
}

var UserAgentBytes []byte = []byte("cryptospeculator")

var JsonParserPool fastjson.ParserPool
var JsonArenaPool fastjson.ArenaPool

type RequestHandle struct {
    JsonParser *fastjson.Parser
    Response *fasthttp.Response
}

// handle http get with json. it returns json value and http status code.
func (rh *RequestHandle) HandleHttpGetJson(httpClient *fasthttp.HostClient,
                host, uri []byte, args *fasthttp.Args) (*fastjson.Value, int) {
    req := fasthttp.AcquireRequest()
    defer fasthttp.ReleaseRequest(req)
    
    if args!=nil {
        // append arguments
        dstUri := make([]byte, 0, len(uri)+10)
        dstUri = append(dstUri, uri...)
        dstUri = append(dstUri, '?')
        dstUri = append(dstUri, args.QueryString()...)
        req.SetRequestURIBytes(dstUri)
    } else {
        req.SetRequestURIBytes(uri)
    }
    if httpClient.IsTLS {   // fix for new fasthttp versions
        req.URI().SetScheme("https")
    }
    req.Header.SetMethod(fasthttp.MethodGet)
    req.SetHostBytes(host)
    req.Header.SetUserAgentBytes(UserAgentBytes)
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Accept-Encoding", "utf-8")
    rh.Response = fasthttp.AcquireResponse()
    if err := httpClient.Do(req, rh.Response); err!=nil {
        ErrorPanic("Error while doing HTTP request", err)
    }
    status := rh.Response.Header.StatusCode()
    if !CheckJsonContentType(rh.Response.Header.ContentType()) {
        // wrong content type (must be json encoded in utf-8
        panic("HTTP response have wrong content-type")
    }
    
    // parse json
    rh.JsonParser = JsonParserPool.Get()
    v, err := rh.JsonParser.ParseBytes(rh.Response.Body())
    if err!=nil {
        ErrorPanic("Error while parsing response", err)
    }
    return v, status
}

// headers - array of string-bytes, even elements are keys, odd are value
func (rh *RequestHandle) HandleHttpPostJson(httpClient *fasthttp.HostClient,
                host, uri, query []byte, body []byte,
                headers [][]byte) (*fastjson.Value, int) {
    req := fasthttp.AcquireRequest()
    defer fasthttp.ReleaseRequest(req)
    
    uriWithQuery := make([]byte, 0, len(uri)+len(query))
    uriWithQuery = append(uriWithQuery, uri...)
    uriWithQuery = append(uriWithQuery, query...)
    req.SetRequestURIBytes(uriWithQuery)
    if httpClient.IsTLS {   // fix for new fasthttp versions
        req.URI().SetScheme("https")
    }
    req.Header.SetMethod(fasthttp.MethodPost)
    req.SetHostBytes(host)
    req.Header.SetUserAgentBytes(UserAgentBytes)
    req.Header.SetContentType("application/json")
    req.Header.SetContentLength(len(body))
    req.Header.Add("Accept", "application/json")
    req.Header.Add("Accept-Encoding", "utf-8")
    
    // set extra headers
    hlen := len(headers)
    for i:=0; i < hlen; i+=2 {
        req.Header.AddBytesKV(headers[i], headers[i+1])
    }
    
    req.SetBody(body)
    
    rh.Response = fasthttp.AcquireResponse()
    if err := httpClient.Do(req, rh.Response); err!=nil {
        ErrorPanic("Error while doing HTTP request", err)
    }
    status := rh.Response.Header.StatusCode()
    if !CheckJsonContentType(rh.Response.Header.ContentType()) {
        // wrong content type (must be json encoded in utf-8
        panic("HTTP response have wrong content-type")
    }
    
    // parse json
    rh.JsonParser = JsonParserPool.Get()
    v, err := rh.JsonParser.ParseBytes(rh.Response.Body())
    if err!=nil {
        ErrorPanic("Error while parsing response", err)
    }
    return v, status
}

// should be called after using request handle
func (rh *RequestHandle) Release() {
    if rh.JsonParser!=nil {
        JsonParserPool.Put(rh.JsonParser)
        rh.JsonParser = nil
    }
    if rh.Response!=nil {
        fasthttp.ReleaseResponse(rh.Response)
        rh.Response = nil
    }
}

/* fastjson utilities */

func FastjsonGetObjectRequired(vx *fastjson.Value) *fastjson.Object {
    if o, err := vx.Object(); err==nil {
        return o
    }
    panic("Wrong json body: no object field")
}

func FastjsonGetString(vx *fastjson.Value) string {
    if vx.Type()==fastjson.TypeNull { return "" }
    if s, err := vx.StringBytes(); err==nil {
        return string(s)
    }
    panic("Wrong json body: no string field")
}

func FastjsonGetBool(vx *fastjson.Value) bool {
    if vx.Type()==fastjson.TypeNull { return false }
    if b, err := vx.Bool(); err==nil {
        return b
    }
    panic("Wrong json body: no bool field")
}

func FastjsonGetStringBytes(vx *fastjson.Value) []byte {
    if vx.Type()==fastjson.TypeNull { return nil }
    if s, err := vx.StringBytes(); err==nil {
        return s
    }
    panic("Wrong json body: no string field")
}

func FastjsonCheckString(vx *fastjson.Value, expected []byte) bool {
    if s, err := vx.StringBytes(); err==nil {
        return bytes.Equal(s, expected)
    }
    panic("Wrong json body: no string field")
}

func FastjsonGetInt(vx *fastjson.Value) int {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if iv, err := vx.Int(); err==nil {
        return iv
    }
    panic("Wrong json body: no integer field")
}

func FastjsonGetUInt(vx *fastjson.Value) uint {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if iv, err := vx.Uint(); err==nil {
        return iv
    }
    panic("Wrong json body: no integer field")
}

func FastjsonGetUInt32(vx *fastjson.Value) uint32 {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if iv, err := vx.Uint(); err==nil {
        if iv > math.MaxUint32 {
            panic("Unsigned integer overflow in json")
        }
        return uint32(iv)
    }
    panic("Wrong json body: no integer field")
}

func FastjsonGetUInt64(vx *fastjson.Value) uint64 {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if iv, err := vx.Uint64(); err==nil {
        return iv
    }
    panic("Wrong json body: no integer field")
}

func FastjsonGetFloat64(vx *fastjson.Value) float64 {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if fv, err := vx.Float64(); err==nil {
        return fv
    }
    panic("Wrong json body: no float field")
}

func FastjsonGetArray(vx *fastjson.Value) []*fastjson.Value {
    if vx.Type()==fastjson.TypeNull { return nil }
    if arr, err := vx.Array(); err==nil {
        return arr
    }
    panic("Wrong json body: no array field")
}

func FastjsonGetUDec128(vx *fastjson.Value, precision uint) godec128.UDec128 {
    if vx.Type()==fastjson.TypeNull { return godec128.UDec128{} }
    if vx.Type()==fastjson.TypeNumber {
        ud, err := godec128.ParseUDec128Bytes(vx.MarshalTo(nil), precision, false)
        if err!=nil {
            panic("Wrong json body: no udec128 field")
        }
        return ud
    }
    panic("Wrong json body: no udec128 field")
}

func FastjsonGetUDec128Signed(vx *fastjson.Value,
                              precision uint) (godec128.UDec128, bool) {
    if vx.Type()==fastjson.TypeNull { return godec128.UDec128{}, false }
    neg := false
    if vx.Type()==fastjson.TypeNumber {
        str := vx.MarshalTo(nil)
        if len(str)>0 && (str[0]=='-' || str[0]=='+') {
            if str[0]=='-' { neg = true }
            str = str[1:]
        }
        ud, err := godec128.ParseUDec128Bytes(str, precision, false)
        if err!=nil {
            panic("Wrong json body: no signed udec128 field")
        }
        return ud, neg
    }
    panic("Wrong json body: no udec128 field")
}

func FastjsonGetUnixTimeMilli(vx *fastjson.Value) time.Time {
    if vx.Type()==fastjson.TypeNull { return time.Time{} }
    if iv, err := vx.Int64(); err==nil {
        return time.Unix(iv/1000, (iv%1000)*1000000)
    }
    panic("Wrong json body: no unix time")
}

func FastjsonGetDuration(vx *fastjson.Value) time.Duration {
    if vx.Type()==fastjson.TypeNull { return 0 }
    if s, err := vx.StringBytes(); err==nil {
        if d, err := time.ParseDuration(string(s)); err!=nil {
            panic("Wrong json body: no time duration field")
        } else {
            return d
        }
    }
    panic("Wrong json body: no time duration field")
}
