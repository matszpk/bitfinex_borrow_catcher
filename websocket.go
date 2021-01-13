/*
 * websocket.go - websocket driver
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
    "errors"
    "fmt"
    "net"
    "net/http"
    "strings"
    "sync"
    "sync/atomic"
    "time"
    "github.com/gorilla/websocket"
)

type ErrorHandler func(error)

type errorHandlerPack struct {
    h ErrorHandler
}

var dummyErrorHandlerPack errorHandlerPack = errorHandlerPack{}

type wsChannelType uint8

type wsFunc func()
type wsDialParamsFunc func() (string, http.Header)
type wsHandleMessageFunc func(msg []byte)
type wsResubscribeChannelFunc func(wsChannelType, interface{})

type websocketDriver struct {
    netDial func(network, addr string) (net.Conn, error)
    dialTrials uint32
    mutex sync.Mutex
    connMutex sync.Mutex
    conn *websocket.Conn
    stopCh chan struct{}
    errCh chan error
    channelsOpened uint32
    errorHandler atomic.Value
    reconnHandler wsFunc
    disconnHandler wsFunc
    resubscribeChannel wsResubscribeChannelFunc
    
    funcRetCh chan string
    funcErrCh chan error
    awaitingFuncRet uint32
    
    callMutex sync.Mutex
    
    dialParams wsDialParamsFunc
    initMessage wsFunc
    handleMessage wsHandleMessageFunc
}

// websocket

// dial routine
func (drv *websocketDriver) dial() (bool, bool) {
    destUrl, header := drv.dialParams()
    
    var dialer websocket.Dialer
    dialer.NetDial = drv.netDial
    dialer.HandshakeTimeout = time.Minute
    
    wsConn, resp, err := dialer.Dial(destUrl, header)
    if err!=nil && (resp==nil || resp.StatusCode==503) {
        return false, true
    }
    if resp.StatusCode >= 400 {
        return false, false
    }
    drv.conn = wsConn
    return true, false
}

// start routine
func (drv *websocketDriver) start() {
    drv.mutex.Lock()
    defer drv.mutex.Unlock()
    
    drv.awaitingFuncRet = 0
    
    if drv.conn!=nil {
        panic("Websocket already started")
    }
    drv.stopCh = make(chan struct{})
    
    drv.connMutex.Lock()
    defer drv.connMutex.Unlock()
    var good, tryAgain bool
    tryAgain = true
    // try 5 times to dial
    for i:=uint32(0); i<drv.dialTrials && tryAgain; i++ {
        good, tryAgain = drv.dial()
        if !good && !tryAgain {
            panic("Can't WSDial")
        }
    }
    if !good { panic("Can't WSDial") }
    
    if drv.initMessage!=nil { drv.initMessage() }
    drv.funcRetCh = make(chan string, 2)
    drv.funcErrCh = make(chan error, 2)
    
    drv.errCh = make(chan error, 2)
    drv.channelsOpened = 1
    
    go drv.handleMessages()
}

// stop websocket
func (drv *websocketDriver) stop() {
    drv.mutex.Lock()
    defer drv.mutex.Unlock()
    
    if atomic.LoadUint32(&drv.awaitingFuncRet)!=0 {
        // break awaiting for function return
        drv.sendErr(drv.funcErrCh,
                    errors.New("Stopping realtime breaks function return"))
    }
    
    /*drv.allTickersHandler.Store(&dummyTickersHandlerPack)
    drv.allDailySummariesHandler.Store(&dummyDailySummariesHandlerPack)
    drv.tickerHandlers = sync.Map{}
    drv.dailySummaryHandlers = sync.Map{}
    drv.tradeHandlers = sync.Map{}
    drv.orderBookHandlers = sync.Map{}
    drv.diffOrderBookHandlers = sync.Map{}
    drv.candleHandlers = sync.Map{}*/
    drv.errorHandler.Store(&dummyErrorHandlerPack)
    drv.reconnHandler = nil
    atomic.StoreUint32(&drv.channelsOpened, 0)
    if drv.conn==nil { return }
    drv.stopCh <- struct{}{}
    close(drv.stopCh)
    if drv.errCh!=nil { close(drv.errCh) }
    drv.conn.Close()
    drv.connMutex.Lock()
    drv.conn = nil
    drv.connMutex.Unlock()
    drv.errCh = nil
    
    if drv.funcRetCh!=nil { close(drv.funcRetCh) }
    if drv.funcErrCh!=nil { close(drv.funcErrCh) }
    drv.funcRetCh = nil
    drv.funcErrCh = nil
    drv.awaitingFuncRet = 0
}

// routine wrapper for catching panics
func (drv *websocketDriver) initMessageSafe() bool {
    good := true
    defer func() {
        if x := recover(); x!=nil {
            good = false
        }
    }()
    if drv.initMessage!=nil { drv.initMessage() }
    return good
}

// replacement of time.Sleep with immediately leaving
func (drv *websocketDriver) reconnectWait(d time.Duration) bool {
    timer := time.NewTimer(d)
    defer timer.Stop()
    select {
        case <- timer.C:
            return true
        case <- drv.stopCh:
            return false
    }
}

// main routine to reconnect
func (drv *websocketDriver) tryReconnect() bool {
    drv.connMutex.Lock()
    defer drv.connMutex.Unlock()
    drv.conn.Close() // force close old connection
    for {
        good, tryAgain := drv.dial()
        if !good && !tryAgain {
            if !drv.reconnectWait(time.Minute) {
                return false
            }
        } else {
            if !drv.reconnectWait(time.Second*10) {
                return false
            }
        }
        if good {
            if !drv.initMessageSafe() {
                continue
            }
            return true
        }
    }
    return false
}

func (drv *websocketDriver) reconnect() bool {
    if drv.disconnHandler!=nil {
        drv.disconnHandler()
    }
    if atomic.LoadUint32(&drv.awaitingFuncRet)!=0 {
        // break awaiting for function return
        drv.sendErr(drv.funcErrCh, errors.New( "Disconnection breaks function return"))
    }
    good := drv.tryReconnect()
    if good {
        go func() {
            drv.resubscribeChannels()
            if drv.reconnHandler!=nil {
                drv.reconnHandler()
            }
        }()
    }
    return good
}

type wsConnMsg struct {
    msg []byte
    code int
}

func (drv *websocketDriver) sendErr(errCh chan<- error, err error) {
    if atomic.LoadUint32(&drv.channelsOpened)!=0 {
        errCh <- err
    }
}

func (drv *websocketDriver) handleMessages() {
    msgCh := make(chan wsConnMsg, 2)
    defer close(msgCh)
    good := true
    var closed uint32 = 0
    
    for good {
        go func() {
            defer func() {
                if x:=recover(); x!=nil {
                    err := errors.New(fmt.Sprint(x))
                    drv.sendErr(drv.errCh, err)
                }
            }()
            drv.connMutex.Lock()  // safely get connection
            conn := drv.conn
            drv.connMutex.Unlock()
            if conn==nil { return }
            // read message from connection
            msgType, msg, err := conn.ReadMessage()
            if err!=nil { drv.sendErr(drv.errCh, err) }
            if atomic.LoadUint32(&closed)==1 { return } // if already closed
            if len(msg)!=0 {
                msgCh <- wsConnMsg{ msg, msgType }
            }
        }()
        
        // dispatch message or error
        select {
            case msg := <-msgCh:
                if msg.code != websocket.PongMessage &&
                    (len(msg.msg)!=2 || msg.msg[0]!='{' || msg.msg[1]!='}') {
                    // this is not a keep-alive message, process
                    go drv.handleMessage(msg.msg)
                }
            case err := <-drv.errCh: {
                Logger.Error("websocket:", err)
                errStr := fmt.Sprint(err)
                if errStr=="repeated read on failed websocket connection" ||
                    strings.LastIndex(errStr, "connection timed out")!=-1 ||
                    websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure,
                            websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                    // abnormal closing
                    good = drv.reconnect()
                } else if (err!=nil) {
                    // other error
                    h := drv.errorHandler.Load().(*errorHandlerPack)
                    if h.h!=nil && err!=nil {
                        go h.h(err)
                    }
                }
            }
            case <-drv.stopCh:
                good = false    // just stop
        }
    }
}

func (drv *websocketDriver) setErrorHandler(h ErrorHandler) {
    if h!=nil { drv.errorHandler.Store(&errorHandlerPack{ h })
    } else { drv.errorHandler.Store(&dummyErrorHandlerPack) }
}

// resubscribe channels after reconnection
func (drv* websocketDriver) resubscribeChannels() {
}
