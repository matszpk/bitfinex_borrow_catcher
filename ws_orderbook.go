/*
 * ws_orderbook.go - websocket orderbook support
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
    "fmt"
    //"sync"
    //"time"
)

type seqNoElem interface {
    seqNo() uint64
    process()
}

type seqHandler interface {
    clear()
    push(nelem seqNoElem) bool
    // check whether initial seqNo is valid for first seqNo
    checkFirstSeqNo(initial, first uint64) bool
    flush() bool
    flushTo(seqNo uint64)
    setFlushHandler(fh func(uint64))
    start()
    stop()
}

/* seqNo handler */
/*
 * SeqNo handling
 * for ordering seqNos and handling holes
 */

const seqNoElemsNum = 200

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

func (nh *seqNoHandler) flushTo(seqNo uint64) {
}

func (nh *seqNoHandler) checkFirstSeqNo(initial, first uint64) bool {
    return initial+1 >= first
}

func (nh *seqNoHandler) setFlushHandler(fh func(uint64)) {
}

func (nh *seqNoHandler) start() {
}

func (nh *seqNoHandler) stop() {
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
        
        i, j := 0, stmpBidLen
        for i<j {
            h := (i+j)>>1
            if diff.Obe.Rate.Cmp(ett[h].Rate) < 0 {
                i = h+1
            } else {
                j = h
            }
        }
        toDelete := diff.Obe.Amount.IsZero()
        
        if i < stmpBidLen {
            sdest.Bid = append(sdest.Bid, ett[:i]...)
            r := diff.Obe.Rate.Cmp(ett[i].Rate)
            if !toDelete {
                sdest.Bid = append(sdest.Bid, diff.Obe)
            }
            if r==0 {
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
    } else {
        // SideOffer
        maxDepth := cap(stmp.Ask)
        ett := stmp.Ask[:]
        stmpAskLen := len(stmp.Ask)
        sdest.Ask = sdest.Ask[:0]
        
        i, j := 0, stmpAskLen
        for i<j {
            h := (i+j)>>1
            if diff.Obe.Rate.Cmp(ett[h].Rate) > 0 {
                i = h+1
            } else {
                j = h
            }
        }
        toDelete := diff.Obe.Amount.IsZero()
        
        if i < stmpAskLen {
            sdest.Ask = append(sdest.Ask, ett[:i]...)
            r := diff.Obe.Rate.Cmp(ett[i].Rate)
            if !toDelete {
                sdest.Ask = append(sdest.Ask, diff.Obe)
            }
            if r==0 {
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
    }
}
