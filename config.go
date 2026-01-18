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
