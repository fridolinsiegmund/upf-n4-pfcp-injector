# 5G UPF N4 PFCP Injector

A Go tool that establishes a PFCP association with a 5G UPF at N4 and installs IPv4 PDU sessions using [go-pfcp](https://github.com/wmnsk/go-pfcp). This way, subscriber PDU sessions are installed in the UPF without the need for a full 5G core and full connection establishment through the core. Sessions use sequential UE addresses starting at `10.60.0.1` and TEIDs starting at `1`.



This tool is a refined version of the tool used in our publication:

F. Siegmund, R. Kundel, T. Meuser, and R. Steinmetz, "User Plane Performance in Beyond 5G Networks: Comprehensive Analysis and Evaluation," Computer Communications, vol. 247, p. 108397, 2026

Note that this is alpha-stage lab tooling, written for the specific measurements in the publication. Expect to adapt it if you target other UPF implementations or parameter ranges than the ones we evaluated.
Contributions that extend the tool to other UPFs and parameters are welcomed.

```
@article{siegmund2026userplane,
  title   = {User Plane Performance in Beyond {5G} Networks: Comprehensive Analysis and Evaluation},
  author  = {Siegmund, Fridolin and Kundel, Ralf and Meuser, Tobias and Steinmetz, Ralf},
  journal = {Computer Communications},
  volume  = {247},
  pages   = {108397},
  year    = {2026},
  issn    = {0140-3664},
  doi     = {10.1016/j.comcom.2025.108397},
  url     = {https://doi.org/10.1016/j.comcom.2025.108397}
}
```


## Requirements

- Go 1.21.0 or newer
- Internet access for the first dependency download
- A reachable UPF configured for your network. The local IPv4 address must be assigned to your machine, with UDP port `8805` available

## Usage

Run from the project directory, replacing the example addresses with your setup:

```bash
go run . \
  -upf_n4_ip=192.168.1.10:8805 \
  -own_ip=192.168.1.20 \
  -upf_n3_ip=10.42.10.1 \
  -gnb_ip=10.42.10.11 \
  -num_sess=100 \
  -mbr_dl=100000 \
  -mbr_ul=100000
```

Dependencies should download automatically. Alternatively, build and run a binary:

```bash
go build -o pfcp-injector .
./pfcp-injector -h
```

## Options

| Flag | Default | Description |
| --- | --- | --- |
| `-upf_n4_ip` | `127.0.0.8:8805` | UPF PFCP destination address and port |
| `-own_ip` | `127.0.0.1` | Local IPv4 address; binds port 8805 |
| `-upf_n3_ip` | `10.42.10.1` | UPF N3 IPv4 address |
| `-gnb_ip` | `10.42.10.11` | gNB IPv4 address |
| `-num_sess` | `10000` | Sessions to request, from 0 to 65278; 0 exits without connecting |
| `-mbr_dl` | `1000000000` | Downlink MBR per session in Kbit/s |
| `-mbr_ul` | `1000000000` | Uplink MBR per session in Kbit/s |
| `-ab_mbr` | `false` | Multiply MBR by 10 for every second session |
| `-request_timeout` | `3s` | Response timeout per attempt |
| `-retries` | `2` | Retransmissions after the initial attempt |

The network instance/DNN is currently fixed to `internet`. Requests are sent sequentially with bounded retries. After the batch, the code prints statistics and keeps answering heartbeats. Press **Ctrl+C** to stop; stopping does not send session deletion requests.

