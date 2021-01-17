/*
 * ws_orderbook.go - websocket orderbook support
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
    "sync"
)

type seqNoElem interface {
    seqNo() uint64
    process()
}

/* seqNo handler */
/*
 * SeqNo handling
 * for ordering seqNos and handling holes
 */

const seqNoElemsNum = 30

type seqNoHandler struct {
    haveSeqNo bool
    startSeqNo uint64
    seqNosNum int
    seqNos [seqNoElemsNum]seqNoElem
}

func (nh *seqNoHandler) clear() {
    nh.haveSeqNo = false
    nh.seqNosNum = 0
    for i:=0; i < seqNoElemsNum; i++ {
        nh.seqNos[i] = nil
    }
}

// return false is some holes in seqNo sequence
func (nh *seqNoHandler) push(nelem seqNoElem) bool {
    neseqNo := nelem.seqNo()
    
    if nh.seqNosNum==0 {
        if !nh.haveSeqNo {
            nh.haveSeqNo = true
            nh.startSeqNo = neseqNo+1
            nelem.process()
            return true
        }
        if neseqNo < nh.startSeqNo {
            Logger.Warn("SeqNo already processed or missed: ",
                        neseqNo, " next: ", nh.startSeqNo)
            return false
        }
        if nh.startSeqNo == neseqNo {
            nh.startSeqNo = neseqNo+1
            nelem.process()
            return true
        } else if neseqNo-nh.startSeqNo < seqNoElemsNum {
            // queue seqNo
            nh.seqNos[neseqNo-nh.startSeqNo] = nelem
            nh.seqNosNum = int(neseqNo-nh.startSeqNo+1)
            return true
        } else {
            // no space in queue
            nh.startSeqNo = neseqNo-seqNoElemsNum+1
            nh.seqNos[neseqNo-nh.startSeqNo] = nelem
            nh.seqNosNum = seqNoElemsNum
            return false
        }
    } else {
        // if have elems in queue
        if neseqNo < nh.startSeqNo {
            Logger.Warn("SeqNo already processed or missed: ",
                        neseqNo, " next: ", nh.startSeqNo)
            return false
        }
        if neseqNo-nh.startSeqNo < seqNoElemsNum {
            // queue seqNo
            if nh.seqNos[neseqNo-nh.startSeqNo]!=nil {
                panic(fmt.Sprint("SeqNo already processed: ", neseqNo))
            }
            nh.seqNos[neseqNo-nh.startSeqNo] = nelem
            var i int
            for i = 0; i < seqNoElemsNum && nh.seqNos[i]!=nil; i++ {
                nh.seqNos[i].process()
            }
            // move back
            nh.startSeqNo += uint64(i)
            nh.seqNosNum -= i
            copy(nh.seqNos[:seqNoElemsNum-i], nh.seqNos[i:])
            // clear rest
            var j int
            for j=seqNoElemsNum-i; j < seqNoElemsNum; j++ {
                nh.seqNos[j] = nil
            }
            // check whether all further entries in queues has been processed
            for j=0; j<seqNoElemsNum-i; j++ {
                if nh.seqNos[j]!=nil { break }
            }
            if j==seqNoElemsNum-i {
                // if all processed
                nh.seqNosNum = 0
                return true
            }
            if nh.seqNosNum < int(neseqNo-nh.startSeqNo+1) {
                nh.seqNosNum = int(neseqNo-nh.startSeqNo+1)
            }
            return true
        } else {
            // no space in queue
            toProcess := int(neseqNo-nh.startSeqNo-seqNoElemsNum+1)
            if toProcess > seqNoElemsNum {
                toProcess = seqNoElemsNum // to next
            }
            // process
            for i := 0; i < toProcess; i++ {
                if nh.seqNos[i]==nil { continue }
                nh.seqNos[i].process()
            }
            nh.startSeqNo += uint64(toProcess)
            copy(nh.seqNos[:seqNoElemsNum-toProcess], nh.seqNos[toProcess:])
            // clear rest
            for j:=seqNoElemsNum-toProcess; j < seqNoElemsNum; j++ {
                nh.seqNos[j] = nil
            }
            if toProcess == seqNoElemsNum {
                nh.startSeqNo = neseqNo-seqNoElemsNum+1
            }
            nh.seqNosNum -= toProcess
            nh.push(nelem)
            return false
        }
    }
}

func (nh *seqNoHandler) flush() bool {
    allProcessed := true
    for i:=0; i < nh.seqNosNum; i++ {
        if nh.seqNos[i]==nil {
            allProcessed = false
            continue
        }
        nh.startSeqNo = nh.seqNos[i].seqNo()
        nh.seqNos[i].process()
        nh.seqNos[i] = nil
    }
    return allProcessed
}

func (nh *seqNoHandler) checkFirstSeqNo(initial, first uint64) bool {
    return initial+1 >= first
}

func (nh *seqNoHandler) setFlushHandler(fh func(uint64)) {
}

// apply orderbook diff

type OrderBookEntryDiff struct {
    Side Side
    Obe OrderBookEntry
}

func (stmp *OrderBook) applyDiff(sdest *OrderBook, diff *OrderBookEntryDiff) {
    if diff.Side == SideBid {
        // SideBid
        maxDepth := cap(stmp.Bid)
        ett := stmp.Bid[:]
        stmpBidLen := len(stmp.Bid)
        sdest.Bid = sdest.Bid[:0]
        
        toDelete := diff.Obe.Rate.IsZero()
        i, j := 0, stmpBidLen
        if !toDelete {
            for i<j {
                h := (i+j)>>1
                if diff.Obe.Cmp(&ett[h]) < 0 {
                    i = h+1
                } else {
                    j = h
                }
            }
        } else {
            for i=0; i < stmpBidLen; i++ {
                if ett[i].Id == diff.Obe.Id {
                    break
                }
            }
        }
        
        if i < stmpBidLen {
            sdest.Bid = append(sdest.Bid, ett[:i]...)
            r := diff.Obe.Cmp(&ett[i])
            if !toDelete {
                sdest.Bid = append(sdest.Bid, diff.Obe)
            }
            if r==0 || toDelete {
                i++ // skip, because replaced or deleted
            }
            xlen := stmpBidLen
            destLen := len(sdest.Bid)
            if xlen > (maxDepth-destLen)+i {
                // correct to maxDepth
                xlen = (maxDepth-destLen)+i
            }
            if i <= stmpBidLen {
                sdest.Bid = append(sdest.Bid, ett[i:xlen]...)
            }
        } else {
            sdest.Bid = append(sdest.Bid, ett...)
            if stmpBidLen < maxDepth && !toDelete {
                sdest.Bid = append(sdest.Bid, diff.Obe)
            }
        }
        
        sdest.Ask = stmp.Ask[:0]
        sdest.Ask = append(sdest.Ask, stmp.Ask...)
    } else {
        // SideOffer
        maxDepth := cap(stmp.Ask)
        ett := stmp.Ask[:]
        stmpAskLen := len(stmp.Ask)
        sdest.Ask = sdest.Ask[:0]
        
        i, j := 0, stmpAskLen
        toDelete := diff.Obe.Rate.IsZero()
        if !toDelete {
            for i<j {
                h := (i+j)>>1
                if diff.Obe.Cmp(&ett[h]) > 0 {
                    i = h+1
                } else {
                    j = h
                }
            }
        } else {
            for i=0; i < stmpAskLen; i++ {
                if ett[i].Id == diff.Obe.Id {
                    break
                }
            }
        }
        
        if i < stmpAskLen {
            sdest.Ask = append(sdest.Ask, ett[:i]...)
            r := diff.Obe.Cmp(&ett[i])
            if !toDelete {
                sdest.Ask = append(sdest.Ask, diff.Obe)
            }
            if r==0 || toDelete {
                i++ // skip, because replaced or deleted
            }
            xlen := stmpAskLen
            destLen := len(sdest.Ask)
            if xlen > (maxDepth-destLen)+i {
                // correct to maxDepth
                xlen = (maxDepth-destLen)+i
            }
            if i <= stmpAskLen {
                sdest.Ask = append(sdest.Ask, ett[i:xlen]...)
            }
        } else {
            sdest.Ask = append(sdest.Ask, ett...)
            if stmpAskLen < maxDepth && !toDelete {
                sdest.Ask = append(sdest.Ask, diff.Obe)
            }
        }
        
        sdest.Bid = stmp.Bid[:0]
        sdest.Bid = append(sdest.Bid, stmp.Bid...)
    }
}

/* small order book update mechanism */

type rtOBPending struct {
    diffSeqNo uint64
    diff *OrderBookEntryDiff
}

type rtOBElem struct {
    diffSeqNo uint64
    diff *OrderBookEntryDiff
    rtob *rtOrderBookHandle
}

func (elem *rtOBElem) seqNo() uint64 {
    return elem.diffSeqNo
}

func (elem *rtOBElem) process() {
    elem.rtob.pendings = append(elem.rtob.pendings,
                        rtOBPending{ elem.diffSeqNo, elem.diff })
}

type rtOrderBookHandle struct {
    name string
    maxDepth int
    mutex sync.Mutex
    initialSeqNo uint64
    initial OrderBook
    haveInitial bool
    snHandler seqNoHandler
    h OrderBookHandler
    pendings []rtOBPending
    toHandlePool sync.Pool
}

func newRtOrderBookHandle(rtName string, fh OrderBookHandler) *rtOrderBookHandle {
    rtob := &rtOrderBookHandle{ name: rtName, maxDepth: 25,
        h: fh, haveInitial: false, pendings: make([]rtOBPending, 0, seqNoElemsNum) }
    rtob.toHandlePool = sync.Pool{ New: rtob.newOrderBooks }
    rtob.initial.Bid = make([]OrderBookEntry, 0, 25)
    rtob.initial.Ask = make([]OrderBookEntry, 0, 25)
    return rtob
}

func (rtob *rtOrderBookHandle) clearPendings() {
    plen := len(rtob.pendings)
    for i:=0; i < plen; i++ {
        rtob.pendings[i] = rtOBPending{}
    }
    rtob.pendings = rtob.pendings[:0]
}

func (rtob *rtOrderBookHandle) pushInitialInt(seqNo uint64,
                        ob *OrderBook) (int, []OrderBook, bool) {
    rtob.mutex.Lock()
    defer rtob.mutex.Unlock()
    rtob.initialSeqNo = seqNo
    rtob.initial.Bid = rtob.initial.Bid[:0]
    rtob.initial.Ask = rtob.initial.Ask[:0]
    rtob.initial.Bid = append(rtob.initial.Bid, ob.Bid...)
    rtob.initial.Ask = append(rtob.initial.Ask, ob.Ask...)
    rtob.haveInitial = true
    return rtob.tryProcessInt()
}

// push initial small order book and try process rest
// return true if current orderbooks to handle updated
func (rtob *rtOrderBookHandle) pushInitial(seqNo uint64, ob *OrderBook) {
    toHandleNum, toHandle, haveOut := rtob.pushInitialInt(seqNo, ob)
    rtob.h(ob)
    rtob.tryProcessEnd(toHandleNum, toHandle, haveOut)
}

// push difference of orderbook
// return false if is not ok and initial orderbook required
func (rtob *rtOrderBookHandle) pushDiffInt(seqNo uint64,
                        diff *OrderBookEntryDiff) (int, []OrderBook, bool, bool) {
    rtob.mutex.Lock()
    defer rtob.mutex.Unlock()
    if !rtob.snHandler.push(&rtOBElem{ seqNo, diff, rtob }) {
        Logger.Warn("Some seqNo has been missed ", rtob.name)
        rtob.snHandler.clear()
        rtob.initial.Bid = rtob.initial.Bid[:0]
        rtob.initial.Ask = rtob.initial.Ask[:0]
        rtob.haveInitial = false
        n, s, ok := rtob.tryProcessInt()
        return n, s, ok, rtob.haveInitial
    }
    n, s, ok := rtob.tryProcessInt()
    return n, s, ok, rtob.haveInitial
}

func (rtob *rtOrderBookHandle) clear() {
    rtob.mutex.Lock()
    defer rtob.mutex.Unlock()
    rtob.snHandler.clear()
    rtob.initial.Bid = rtob.initial.Bid[:0]
    rtob.initial.Ask = rtob.initial.Ask[:0]
    rtob.haveInitial = false
    rtob.clearPendings()
}

func (rtob *rtOrderBookHandle) pushDiff(seqNo uint64, diff *OrderBookEntryDiff) bool {
    toHandleNum, toHandle, haveOut, needInitial := rtob.pushDiffInt(seqNo, diff)
    rtob.tryProcessEnd(toHandleNum, toHandle, haveOut)
    return needInitial
}

func (rtob *rtOrderBookHandle) newOrderBooks() interface{}  {
    out := make([]OrderBook, seqNoElemsNum)
    for i:=0; i < seqNoElemsNum; i++ {
        out[i].Bid = make([]OrderBookEntry, 0, rtob.maxDepth)
        out[i].Ask = make([]OrderBookEntry, 0, rtob.maxDepth)
    }
    return out
}

func (rtob *rtOrderBookHandle) tryProcessInt() (int, []OrderBook, bool) {
    pendingsLen := len(rtob.pendings)
    if pendingsLen==0 || !rtob.haveInitial { // if no pendings or no initial
        return 0, nil, false
    }
    if !rtob.snHandler.checkFirstSeqNo(rtob.initialSeqNo, rtob.pendings[0].diffSeqNo) {
        // no initial is too old
        rtob.initial.Bid = rtob.initial.Bid[:0]
        rtob.initial.Ask = rtob.initial.Ask[:0]
        rtob.haveInitial = false
        return 0, nil, false
    }
    i := 0
    for ; i < pendingsLen; i++ {
        if rtob.initialSeqNo < rtob.pendings[i].diffSeqNo {
            break
        }
    }
    if i==pendingsLen { // if initial is too new
        rtob.clearPendings() // make empty
        return 0, nil, false
    }
    // real process
    toHandle := rtob.toHandlePool.Get().([]OrderBook)
    defer func() {
        if x:=recover(); x!=nil {
            rtob.toHandlePool.Put(toHandle)
            panic(x) // panic again
        }
    }()
    // process pendings
    toHandleNum := 0
    rtob.initial.applyDiff(&toHandle[0], rtob.pendings[i].diff)
    j:=i+1
    for ; j < pendingsLen; j++ {
        toHandle[j-i-1].applyDiff(&toHandle[j-i], rtob.pendings[j].diff)
    }
    toHandleNum = j-i
    rtob.initial.Bid = rtob.initial.Bid[:0]
    rtob.initial.Ask = rtob.initial.Ask[:0]
    rtob.initial.Bid = append(rtob.initial.Bid, toHandle[toHandleNum-1].Bid...)
    rtob.initial.Ask = append(rtob.initial.Ask, toHandle[toHandleNum-1].Ask...)
    rtob.initialSeqNo = rtob.pendings[j-1].diffSeqNo
    // clearing pendings
    rtob.clearPendings()
    return toHandleNum, toHandle, true
}

func (rtob *rtOrderBookHandle) tryProcessEnd(toHandleNum int,
                                toHandle []OrderBook, haveOut bool) {
    if !haveOut { return }
    defer rtob.toHandlePool.Put(toHandle)
    if toHandleNum>1 { // call concurrently
        for i:=0; i < toHandleNum; i++ {
            go rtob.h(&toHandle[i])
        }
    } else if toHandleNum>0 {
        rtob.h(&toHandle[0])
    }
}
