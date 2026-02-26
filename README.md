# Scouter Server (Go)

Scouter APM의 **경량 백엔드 서버**입니다. 기존 Java 서버를 Go로 재작성하여 메모리·CPU 효율을 대폭 개선하면서 동일한 APM 데이터 수집 기능을 제공합니다. 단일 바이너리로 배포되며, JVM 없이 즉시 실행할 수 있습니다.

## Java 대비 효율 비교

| 항목 | Java (Scouter 2.21.2) | Go | 비고 |
|------|----------------------|-----|------|
| **런타임 메모리** | 400~800 MB (JVM 힙 + Metaspace) | 50~150 MB (GOMEMLIMIT=1GiB) | **70~80% 절감** |
| **배포 크기** | ~34 MB (server/lib/ JAR 전체) + JRE | 7.4 MB (단일 바이너리, stripped) | **JRE 불필요** |
| **시작 시간** | 수 초 (JVM 클래스 로딩) | ~50ms | **즉시 시작** |
| **GC 정지** | Stop-the-World (G1GC 수십~수백 ms) | 동시 수집 GC (sub-ms STW) | **지연 스파이크 감소** |
| **XLog 수집 처리량** | - | ~27 ns/op (단일), ~11 ns/op (병렬) | 0~1 alloc/op |
| **디스크 I/O (쓰기)** | - | 768 MB/s (1KB 레코드, 0 alloc/op) | buffered + pwrite |
| **인메모리 해시 조회** | - | 14 ns/op (0 alloc/op) | MemHashBlock |

### 메모리 최적화 기법

- **Zero-allocation 직렬화**: DataOutputX의 WriteInt16/32/64 등이 buffer 모드에서 heap 할당 없이 동작 (escape analysis 검증 완료)
- **sync.Pool 기반 버퍼 재사용**: zstd 디코드 버퍼, XLogPack 직렬화 버퍼 등을 풀링하여 GC 부담 최소화
- **Zero-copy 역직렬화**: ReadBlobRef로 내부 버퍼를 복사 없이 참조
- **RWMutex 기반 읽기 최적화**: MemHashBlock/MemTimeBlock의 읽기 경로에서 lock 경합 최소화
- **GOMEMLIMIT**: 런타임 메모리 상한 1GiB 설정으로 OOM 방지 및 GC 효율 최적화

## Features

- **경량 실행 환경**: JVM 불필요, 단일 바이너리 배포, 낮은 메모리 사용량 (Java 서버 대비 ~70% 절감)
- TCP/UDP 기반 에이전트 데이터 수집 (기본 포트: 6100)
- XLog, Counter, Profile, Alert, Summary 등 APM 데이터 처리
- 인메모리 캐시 및 디스크 기반 스토리지
- REST API 서버 (기본 포트: 6180, 설정으로 활성화)
- 설정 파일 핫 리로드 지원
- 일 단위 데이터 보관 및 자동 삭제

## Installation

### Release 바이너리 (권장)

[GitHub Releases](https://github.com/scouter-project/scouter-server-go/releases)에서 플랫폼에 맞는 패키지를 다운로드합니다.

| 파일 | 플랫폼 |
|------|--------|
| scouter-server-linux-amd64.tar.gz | Linux x86_64 |
| scouter-server-linux-arm64.tar.gz | Linux ARM64 |
| scouter-server-darwin-amd64.tar.gz | macOS x86_64 |
| scouter-server-darwin-arm64.tar.gz | macOS ARM64 (Apple Silicon) |
| scouter-server-windows-amd64.zip | Windows x86_64 |

#### Linux / macOS

```bash
# 다운로드 및 압축 해제 (예: linux-amd64)
tar xzf scouter-server-linux-amd64.tar.gz
cd scouter-server-linux-amd64

# 설정 파일 편집
vi conf/scouter.conf

# 실행
./start.sh
```

배포 패키지 구조:

```
scouter-server-linux-amd64/
├── scouter-server          # 실행 바이너리
├── conf/
│   └── scouter.conf        # 설정 파일
├── start.sh                # 시작 스크립트
├── stop.sh                 # 종료 스크립트
└── scouter-server.service  # systemd 유닛 파일 (Linux)
```

#### Windows

```
# 압축 해제 후
cd scouter-server-windows-amd64

# 설정 파일 편집
notepad conf\scouter.conf

# 실행
start.bat
```

#### systemd 서비스 등록 (Linux)

```bash
# 바이너리를 /opt에 배치
sudo cp -r scouter-server-linux-amd64 /opt/scouter-server

# scouter 사용자 생성
sudo useradd -r -s /sbin/nologin scouter
sudo chown -R scouter:scouter /opt/scouter-server

# 서비스 등록 및 시작
sudo cp /opt/scouter-server/scouter-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable scouter-server
sudo systemctl start scouter-server

# 상태 확인
sudo systemctl status scouter-server
```

### 소스에서 빌드

Go 1.26+ 필요

```bash
git clone https://github.com/scouter-project/scouter-server-go.git
cd scouter-server-go

# 빌드
make build

# 크로스 컴파일 (linux, darwin, windows)
make build-all

# 배포 패키지 생성 (바이너리 + 설정 + 스크립트)
make dist-all

# 테스트
make test

# 전체 명령어 확인
make help
```

빌드 결과물은 `dist/` 디렉토리에 생성됩니다.

## Configuration

설정 파일 경로: `./conf/scouter.conf` (환경변수 `SCOUTER_CONF`로 변경 가능)

| 설정 | 기본값 | 설명 |
|------|--------|------|
| `net_udp_listen_port` | 6100 | UDP 에이전트 데이터 수신 포트 |
| `net_tcp_listen_port` | 6100 | TCP 에이전트/클라이언트 연결 포트 |
| `net_http_port` | 6180 | REST API 포트 |
| `net_http_enabled` | false | HTTP API 활성화 여부 |
| `db_dir` | `./database` | 데이터 저장소 경로 |
| `db_keep_days` | 30 | 데이터 보관 일수 |
| `log_dir` | `./logs` | 로그 디렉토리 |

### 메모리 설정

`GOMEMLIMIT` 환경변수로 런타임 메모리 상한을 설정합니다. `start.sh`의 기본값은 `1GiB`입니다.

```bash
export GOMEMLIMIT=2GiB    # 2GB로 변경
./start.sh
```

## Run

```bash
# make로 빌드 후 실행
make run

# 또는 직접 실행
./dist/scouter-server

# 버전 확인
./dist/scouter-server version
```

### 종료

PID 파일(`{PID}.scouter`)을 삭제하면 서버가 graceful shutdown됩니다.

```bash
# start.sh로 실행한 경우
./stop.sh

# 또는 SIGTERM/SIGINT
kill <PID>
```

## Documentation

- [통신 프로토콜 개요](docs/protocol-overview.md) — 바이너리 직렬화, UDP/TCP 패킷 구조, Pack/Value 타입 체계
- [TCP 에이전트 프로토콜 상세](docs/tcp-agent-protocol.md) — 에이전트 연결 수립, 커넥션 풀, Keepalive, RPC 호출 패턴
- [Text Cache Database](docs/text-cache-database.md) — 해시 기반 텍스트 저장소, 3계층 캐시 구조, 디스크 파일 포맷, 일별 로테이션
- [XLog Pipeline](docs/xlog-pipeline.md) — XLog 수신/처리/저장/조회 파이프라인, 링 버퍼 실시간 스트리밍, 3중 인덱스, 서비스 그룹 집계

## License

Apache-2.0
