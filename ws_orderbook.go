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
        
        toDelete := diff.Obe.Rate == 0
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
        toDelete := diff.Obe.Rate == 0
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

type rtOrderBookHandle struct {
    name string
    maxDepth int
    initial OrderBook
    haveInitial bool
    h OrderBookHandler
}

func newRtOrderBookHandle(rtName string, fh OrderBookHandler) *rtOrderBookHandle {
    rtob := &rtOrderBookHandle{ name: rtName, maxDepth: 25,
        h: fh, haveInitial: false }
    rtob.initial.Bid = make([]OrderBookEntry, 0, 25)
    rtob.initial.Ask = make([]OrderBookEntry, 0, 25)
    return rtob
}

func (rtob *rtOrderBookHandle) clear() {
    rtob.initial.Bid = make([]OrderBookEntry, 0, 25)
    rtob.initial.Ask = make([]OrderBookEntry, 0, 25)
    rtob.haveInitial = false
}

// push initial small order book and try process rest
// return true if current orderbooks to handle updated
func (rtob *rtOrderBookHandle) pushInitial(ob *OrderBook) {
    rtob.haveInitial = true
    rtob.initial.copyFrom(ob)
    go rtob.h(ob)
}

func (rtob *rtOrderBookHandle) pushDiff(diff *OrderBookEntryDiff) {
    var ob OrderBook
    rtob.initial.applyDiff(&ob, diff)
    rtob.initial.copyFrom(&ob)
    go rtob.h(&ob)
}
