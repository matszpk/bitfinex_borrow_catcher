/*
 * bitfinex_rt_public.go - Bitfinex Realtime Public client
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
    "bytes"
    "errors"
    "fmt"
    "sync"
    "sync/atomic"
    //"time"
    "net/http"
    "github.com/gorilla/websocket"
    //"github.com/matszpk/godec128"
    "github.com/valyala/fastjson"
)

var (
    bitfinexSocketConnectUrl = "wss://api-pub.bitfinex.com/ws/2"
    bitfinexSocketConnectUrlTest = "ws://api-pub.bitfinex.com/ws/2"
    bitfinexStrEvent = []byte("event")
    bitfinexStrChanId = []byte("chanId")
    bitfinexStrMsg = []byte("msg")
    bitfinexStrWSConfig = []byte(`{"event":"conf","flags":65536}`)
)

type BitfinexRTPublic struct {
    websocketDriver
    wsChannelMap sync.Map
    wsTradeChanIdMap map[string]string
    wsOrderBookChanIdMap map[string]string
    wsOrderBookBrokenMap sync.Map
}

type bitfinexChannelEntry struct {
    channelType wsChannelType
    key interface{}
    firstMsgs [][]byte
}

func NewBitfinexRTPublic() *BitfinexRTPublic {
    drv := &BitfinexRTPublic{}
    drv.dialTrials = 5
    drv.dialParams = drv.wsDialParams
    drv.lateInit = drv.wsLateInit
    drv.initMessage = drv.wsInitMessage
    drv.handleMessage = drv.wsHandleMessage
    drv.resubscribeChannel = drv.wsResubscribeChannel
    return drv
}

func (drv *BitfinexRTPublic) wsDialParams() (string, http.Header)  {
    header := make(http.Header)
    header.Add("User-Agent", string(UserAgentBytes))
    return bitfinexSocketConnectUrl, header
}

func (drv *BitfinexRTPublic) wsInitMessage() {
    // event info
    msgType, msg, err := drv.conn.ReadMessage()
    if err!=nil {
        ErrorPanic("Can't read info message", err)
    }
    if msgType!=websocket.TextMessage{ panic("Message type is not CodeText") }
    
    // set websocket configuration - add sequence number
    drv.conn.WriteMessage(websocket.TextMessage, bitfinexStrWSConfig)
    msgType, msg, err = drv.conn.ReadMessage()
    if err!=nil {
        ErrorPanic("Can't read config response message", err)
    }
    if msgType!=websocket.TextMessage{ panic("Message type is not CodeText") }
    jp := JsonParserPool.Get()
    defer JsonParserPool.Put(jp)
    msgv, err := jp.ParseBytes(msg)
    // check response
    if err!=nil {
        ErrorPanic("Can't parse setconfig response", err)
    }
    msgo := FastjsonGetObjectRequired(msgv)
    statusv := msgo.Get("status")
    if statusv==nil { panic("Wrong response after configuration") }
    if FastjsonGetString(statusv)!="OK" {
        panic("Wrong response after configuration")
    }
}

func (drv *BitfinexRTPublic) wsLateInit() {
    drv.wsChannelMap = sync.Map{}
    drv.wsTradeChanIdMap = make(map[string]string)
    drv.wsOrderBookChanIdMap = make(map[string]string)
    drv.wsOrderBookBrokenMap = sync.Map{}
}

func (drv *BitfinexRTPublic) wsHandleMessage(msg []byte) {
    defer func() {
        if x:=recover(); x!=nil {
            drv.sendErr(drv.errCh, errors.New(fmt.Sprint("Fatal error: ", x)))
        }
    }()
    
    jp := JsonParserPool.Get()
    defer JsonParserPool.Put(jp)
    msgv, err := jp.ParseBytes(msg)
    if err!=nil {
        drv.sendErr(drv.errCh, err)
        return
    }
    
    if msgv.Type() == fastjson.TypeArray {
        // get channel message
        var arr []*fastjson.Value
        if arr, err = msgv.Array(); err!=nil {
            drv.sendErr(drv.errCh, err)
            return
        }
        if len(arr) < 2 {
            drv.sendErr(drv.errCh, errors.New("Wrong channel message"))
            return
        }
        if arr[1].Type()==fastjson.TypeString && FastjsonGetString(arr[1])=="hb" {
            return  // ignore heartbeat
        }
        chanId := string(arr[0].MarshalTo(nil))
        // check channel
        v, ok := drv.wsChannelMap.LoadOrStore(chanId, &bitfinexChannelEntry{
                            firstMsgs: [][]byte{msg} })
        if ok { // if already initialized, handle message
            channEntry := v.(*bitfinexChannelEntry)
            if channEntry.key!=nil {
                drv.handleChannelMessage(channEntry.channelType, channEntry.key, arr)
            } else {
                // not ready just add next firstMsg
                channEntry.firstMsgs = append(channEntry.firstMsgs, msg)
            }
        }
    } else {
        // get command (function) message
        var msgo *fastjson.Object
        if msgo, err = msgv.Object(); err!=nil {
            drv.sendErr(drv.funcErrCh, err)
            return
        }
        // get fields
        var eventStr, msgStr, chanIdStr string
        mask := 0
        msgo.Visit(func(key []byte, vx *fastjson.Value) {
            if (mask&1)==0 && bytes.Equal(key, bitfinexStrEvent) {
                eventStr = FastjsonGetString(vx)
                mask |= 1
            }
            if (mask&2)==0 && bytes.Equal(key, bitfinexStrChanId) {
                chanIdStr = string(vx.MarshalTo(nil))
                mask |= 2
            }
            if (mask&4)==0 && bytes.Equal(key, bitfinexStrMsg) {
                msgStr = FastjsonGetString(vx)
                mask |= 4
            }
        })
        
        if eventStr!="error" {
            drv.sendFuncRet(chanIdStr)  // send channel id
        } else {
            drv.sendErr(drv.funcErrCh, errors.New(
                            fmt.Sprint("Bitfinex command error: ", msgStr)))
        }
    }
}

func (drv *BitfinexRTPublic) handleChannelMessage(chType wsChannelType,
                        keyObj interface{}, arr []*fastjson.Value) {
}

// routine to handle message from stored message in bytes
func (drv *BitfinexRTPublic) handleChannelMessageString(chType wsChannelType,
                        keyObj interface{}, msg []byte) {
    jp := JsonParserPool.Get()
    defer JsonParserPool.Put(jp)
    msgv, err := jp.ParseBytes(msg)
    if err!=nil {
        drv.sendErr(drv.errCh, err)
        return
    }
    var arr []*fastjson.Value
    if arr, err = msgv.Array(); err!=nil {
        drv.sendErr(drv.errCh, err)
        return
    }
    drv.handleChannelMessage(chType, keyObj, arr)
}

func (drv *BitfinexRTPublic) StartRealtime() {
    drv.start()
}

func (drv *BitfinexRTPublic) StopRealtime() {
    drv.stop()
    drv.wsChannelMap = sync.Map{}
    drv.wsTradeChanIdMap = nil
    drv.wsOrderBookChanIdMap = nil
    drv.wsOrderBookBrokenMap = sync.Map{} // clear map
}

func (drv *BitfinexRTPublic) handleCommand(cmdBytes []byte) string {
    drv.sendCommand(cmdBytes)
    atomic.StoreUint32(&drv.awaitingFuncRet, 1)
    defer atomic.StoreUint32(&drv.awaitingFuncRet, 0)
    select {
        case ret := <-drv.funcRetCh:
            return ret
        case err := <-drv.funcErrCh:
            if err!=nil {
                ErrorPanic("Bittrex function error: ", err)
            }
    }
    return ""
}

var bitfinexCmdUnsubscribe0 = []byte(`{"event":"unsubscribe","chanId":`)

var bitfinexCmdSubscribeTicker0 = []byte(
                `{"event":"subscribe","channel":"ticker","symbol":"t`)
var bitfinexCmdEnd0 = []byte(`"}`)

// add channel to wsChannelMap and handle first messages if enabled (callFirsts)
func (drv *BitfinexRTPublic) wsAddChannel(chanId string, chType wsChannelType,
                            keyObj interface{}, callFirsts bool) {
    obj, ok := drv.wsChannelMap.LoadOrStore(chanId, &bitfinexChannelEntry{
            channelType: chType, key: keyObj, firstMsgs: nil })
    if ok {
        // already first message receive
        chanEntry := obj.(*bitfinexChannelEntry)
        chanEntry.channelType = chType
        chanEntry.key = keyObj
        msgs := chanEntry.firstMsgs
        chanEntry.firstMsgs = nil
        // handle first message if choosen (callFirsts)
        if callFirsts {
            go func() {
                for _, msg := range msgs {
                    drv.handleChannelMessageString(chanEntry.channelType,
                                            chanEntry.key, msg)
                }
            }()
        }
    }
}

var bitfinexCmdSubscribeTrades0 = []byte(
                `{"event":"subscribe","channel":"trades","symbol":"f`)

func bitfinexUnsubscribeCmd(chanId string) []byte {
    cmdBytes := make([]byte, 0, 50)
    cmdBytes = append(cmdBytes, bitfinexCmdUnsubscribe0...)
    cmdBytes = append(cmdBytes, chanId...)
    cmdBytes = append(cmdBytes, '}')
    return cmdBytes
}

// internal routine SubscribeTrades (for resubscription after reconnection)
func (drv *BitfinexRTPublic) subscribeTradesInt(market string, h TradeHandler) {
    cmdBytes := make([]byte, 0, 60)
    cmdBytes = append(cmdBytes, bitfinexCmdSubscribeTrades0...)
    cmdBytes = append(cmdBytes, market...)
    cmdBytes = append(cmdBytes, bitfinexCmdEnd0...)
    chanId := drv.handleCommand(cmdBytes)
    if h!=nil { // conditional used by resubscription after reconnection
        drv.setTradeHandler(market, h)
    }
    
    drv.wsTradeChanIdMap[market] = chanId
    drv.wsAddChannel(chanId, wsTrades, market, false)
}

func (drv *BitfinexRTPublic) SubscribeTrades(market string, h TradeHandler) {
    drv.callMutex.Lock()
    defer drv.callMutex.Unlock()
    drv.subscribeTradesInt(market, h)
}

func (drv *BitfinexRTPublic) UnsubscribeTrades(market string) {
    drv.callMutex.Lock()
    defer drv.callMutex.Unlock()
    
    chanId := drv.wsTradeChanIdMap[market]
    drv.handleCommand(bitfinexUnsubscribeCmd(chanId))
    drv.unsetTradeHandler(market)
    
    delete(drv.wsTradeChanIdMap, market)
    drv.wsChannelMap.Delete(chanId)
}

var bitfinexCmdSubscribeOrderBook0 = []byte(
                `{"event":"subscribe","channel":"book","symbol":"t`)
var bitfinexCmdSubscribeOrderBooEnd0 = []byte(`","freq":"F0","prec":"P0","len":"25"}`)

func bitfinexSubscribeOrderBookCmd(market string) []byte {
    cmdBytes := make([]byte, 0, 60)
    cmdBytes = append(cmdBytes, bitfinexCmdSubscribeOrderBook0...)
    cmdBytes = append(cmdBytes, market...)
    cmdBytes = append(cmdBytes, bitfinexCmdSubscribeOrderBooEnd0...)
    return cmdBytes
}

// internal routine SubscribeOrderBook (for resubscription after reconnection)
func (drv *BitfinexRTPublic) subscribeOrderBookInt(market string, h OrderBookHandler) {
    drv.wsOrderBookBrokenMap.Delete(market)
    
    chanId := drv.handleCommand(bitfinexSubscribeOrderBookCmd(market))
    if h!=nil { // conditional used by resubscription after reconnection
        drv.setDiffOrderBookHandler(market, h)
    }
    
    drv.wsOrderBookChanIdMap[market] = chanId
    drv.wsAddChannel(chanId, wsDiffOrderBook, market, true)
}

func (drv *BitfinexRTPublic) SubscribeOrderBook(market string, h OrderBookHandler) {
    drv.callMutex.Lock()
    defer drv.callMutex.Unlock()
    drv.subscribeOrderBookInt(market, h)
}

func (drv *BitfinexRTPublic) UnsubscribeOrderBook(market string) {
    drv.callMutex.Lock()
    defer drv.callMutex.Unlock()
    
    chanId := drv.wsOrderBookChanIdMap[market]
    drv.handleCommand(bitfinexUnsubscribeCmd(chanId))
    drv.unsetDiffOrderBookHandler(market)
    
    delete(drv.wsOrderBookChanIdMap, market)
    drv.wsChannelMap.Delete(chanId)
    drv.wsOrderBookBrokenMap.Delete(market)
}

func (drv *BitfinexRTPublic) wsResubscribeChannel(chType wsChannelType, key string) {
    switch chType {
        case wsInitialize:
            drv.wsLateInit()
        case wsTrades:
            drv.subscribeTradesInt(key, nil)
        case wsDiffOrderBook:
            drv.getDiffOrderBookHandle(key).clear()
            drv.subscribeOrderBookInt(key, nil)
    }
}
