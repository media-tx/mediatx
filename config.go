// Copyright (c) 2026 MediaTX
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

// RTMPConfig holds the configuration for RTMP/SRT server
type RTMPConfig struct {
	TCP struct {
		Addr         string `json:"addr"`
		MaxQueue     int    `json:"maxQueue"`
		Workers      int    `json:"workers"`
		MaxConns     int    `json:"maxConns"`
		ReadTimeout  string `json:"readTimeout"`
		WriteTimeout string `json:"writeTimeout"`
	} `json:"tcp"`
	UDP struct {
		Addr        string `json:"addr"`
		MaxQueue    int    `json:"maxQueue"`
		Workers     int    `json:"workers"`
		MaxPPS      int    `json:"maxPPS"`
		BufferSize  int    `json:"bufferSize"`
		ReadTimeout string `json:"readTimeout"`
	} `json:"udp"`
	SRT struct {
		Addr         string `json:"addr"`
		MaxQueue     int    `json:"maxQueue"`
		Workers      int    `json:"workers"`
		MaxConns     int    `json:"maxConns"`
		Latency      int    `json:"latency"`
		Passphrase   string `json:"passphrase"`
		PbKeyLen     int    `json:"pbkeylen"`
		ReadTimeout  string `json:"readTimeout"`
		WriteTimeout string `json:"writeTimeout"`
	} `json:"srt"`
}
