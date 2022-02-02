/*
 * auth.go - authentication
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
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/hex"
    "io"
    "io/ioutil"
    "os"
    "time"
    "golang.org/x/crypto/argon2"
    "github.com/chzyer/readline"
)

var argon2Salt = []byte("vv9re$Tbvwds@WSg82d1")
var argon2KeySalt = []byte("ktyg9g4$GVw89cf4T@1qfyh3")

const (
    argon2TimeCost = 5
    argon2MemCost = 2*1024
    argon2Parallel = 1
    argon2HashLength = 64
)

const pricePeriod = time.Minute

func passwordHash(password []byte) []byte {
    return argon2.IDKey(password, argon2Salt, argon2TimeCost,
                    argon2MemCost, argon2Parallel, argon2HashLength)
}

func passwordKeyHash(password []byte) []byte {
    return argon2.IDKey(password, argon2KeySalt, argon2TimeCost,
                    argon2MemCost, argon2Parallel, argon2HashLength)
}

// return password hash
func GetPasswordFile(passwordFile string) []byte {
    // get password hash from file
    if content, err := ioutil.ReadFile(passwordFile); err==nil {
        if len(content) < 2*argon2HashLength {
            panic("Wrong length of password file")
        }
        content = content[:2*argon2HashLength]
        passwordHash := make([]byte, argon2HashLength)
        if _, err = hex.Decode(passwordHash, content); err!=nil {
            ErrorPanic("Can't decode Password hash", err)
        }
        return passwordHash
    } else {
        ErrorPanic("Can't read password hash file", err)
    }
    return nil
}

func genAESKey(password []byte) []byte {
    var aesKey [32]byte
    plen := len(password)
    for i := 0; i < plen; i+=32 {
        for j := 0; j < 32 && i+j < plen; j++ {
            aesKey[j] ^= password[i+j]
        }
    }
    return aesKey[:]
}

func encryptExchAuth(passwordHash, apiKey, secretKey []byte) []byte {
    key := genAESKey(passwordHash)
    var iv [aes.BlockSize]byte
    if _, err := io.ReadFull(rand.Reader, iv[:]); err!=nil {
        ErrorPanic("Can't generate IV", err)
    }
    if aesCiph, err := aes.NewCipher(key); err==nil {
         blkMode := cipher.NewCBCEncrypter(aesCiph, iv[:])
         // create text plain
         apiKeyLen, secretKeyLen := len(apiKey), len(secretKey)
         totLen := 4 + apiKeyLen + secretKeyLen
         ciphLen := ((totLen + aes.BlockSize-1) / aes.BlockSize) * aes.BlockSize
         textPlain := make([]byte, ciphLen + aes.BlockSize)
         textPlain[0] = byte(apiKeyLen&0xff)
         textPlain[1] = byte(apiKeyLen>>8)
         copy(textPlain[2:2+apiKeyLen], apiKey)
         textPlain[2+apiKeyLen] = byte(secretKeyLen&0xff)
         textPlain[3+apiKeyLen] = byte(secretKeyLen>>8)
         copy(textPlain[4+apiKeyLen:], secretKey)
         for i := 0; i < aes.BlockSize; i++ {
             textPlain[ciphLen+i] = 117
         }
         
         ciphOut := make([]byte, ciphLen + 2*aes.BlockSize)
         copy(ciphOut[:aes.BlockSize], iv[:])
         blkMode.CryptBlocks(ciphOut[aes.BlockSize:], textPlain)
         return ciphOut
    } else {
        ErrorPanic("Can't create AES cipher", err)
    }
    return nil
}

func decryptExchAuth(passwordHash, ciphData []byte) ([]byte, []byte) {
    key := genAESKey(passwordHash)
    iv := ciphData[:aes.BlockSize]
    if aesCiph, err := aes.NewCipher(key); err==nil {
        blkMode := cipher.NewCBCDecrypter(aesCiph, iv)
        ciphText := ciphData[aes.BlockSize:]
        ciphLen := len(ciphText)
        plainData := make([]byte, ciphLen)
        blkMode.CryptBlocks(plainData, ciphText)
        
        for i := 0; i < aes.BlockSize; i++ {
            if plainData[ciphLen - aes.BlockSize + i] != 117 {
                panic("Wrong password to decrypt exchange auth file")
            }
        }
        
        apiKeyLen := int(plainData[0]) + (int(plainData[1])<<8)
        if apiKeyLen+2 > ciphLen - aes.BlockSize {
            panic("Wrong data in exchange auth file")
        }
        secretKeyLen := int(plainData[2+apiKeyLen]) + (int(plainData[3+apiKeyLen])<<8)
        if apiKeyLen + secretKeyLen + 4 > ciphLen - aes.BlockSize {
            panic("Wrong data in exchange auth file")
        }
        
        apiKey := plainData[2:2+apiKeyLen]
        secretKey := plainData[4+apiKeyLen:4+apiKeyLen+secretKeyLen]
        return apiKey, secretKey
    } else {
        ErrorPanic("Can't create AES cipher", err)
    }
    return nil, nil
}

func AuthenticateExchange(config *Config) ([]byte, []byte) {
    return authenticateExchangeInt(config, readline.Password)
}

func authenticateExchangeInt(config *Config,
                             rdpwd func(string) ([]byte, error)) ([]byte, []byte) {
    expPasswordHash := GetPasswordFile(config.PasswordFile)
    pwd, err := rdpwd("Enter password:")
    if err!=nil {
        ErrorPanic("Can't read password", err)
    }
    
    pwdHash := passwordHash(pwd)
    if !bytes.Equal(expPasswordHash, pwdHash[:]) {
        panic("Wrong password")
    }
    
    pwdKeyHash := passwordKeyHash(pwd)
    
    if exauthRaw, err := ioutil.ReadFile(config.AuthFile); os.IsNotExist(err) {
        // if file doesn't exist
        apiKey, err := rdpwd("Enter APIKey:")
        if err!=nil {
            ErrorPanic("Can't read APIKey", err)
        }
        secretKey, err := rdpwd("Enter SecretKey:")
        if err!=nil {
            ErrorPanic("Can't read SecretKey", err)
        }
        
        // write to exchange auth file
        data := encryptExchAuth(pwdKeyHash, apiKey, secretKey)
        if err =  ioutil.WriteFile(config.AuthFile, data, 0600); err!=nil {
            ErrorPanic("Can't write exchange auth file", err)
        }
        return apiKey, secretKey
    } else if err!=nil {
        ErrorPanic("Can't read exchange auth file", err)
        return nil, nil
    } else {
        // read from exchange
        return decryptExchAuth(pwdKeyHash, exauthRaw)
    }
}

func GenPassword(filename string) {
    genPasswordInt(filename, readline.Password)
}

func genPasswordInt(filename string, rdpwd func(string) ([]byte, error)) {
    pwd, err := rdpwd("Enter password:")
    if err!=nil {
        ErrorPanic("Can't read password", err)
    }
    confirmPwd, err := rdpwd("Confirm password:")
    if err!=nil {
        ErrorPanic("Can't read password", err)
    }
    if !bytes.Equal(pwd, confirmPwd) {
        panic("Password mismatch!")
    }
    
    pwdHash := passwordHash(pwd)
    pwdHashHex := make([]byte, len(pwdHash)*2)
    hex.Encode(pwdHashHex, pwdHash)
    if err := ioutil.WriteFile(filename, pwdHashHex, 0600); err!=nil {
        ErrorPanic("Can't write password to file", err)
    }
}
