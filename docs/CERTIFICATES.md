# 签名 + 加密密钥全梳理

本文档列出 atome-fin-go-sdk 在 **HTTP 签名** 与 **AES 混合加密** 两条链路上涉及的全部密钥：谁持有、谁提供、SDK 配置名、代码位置、适用接口。

---

## 1. 总览：两套 RSA 证书对 + 临时 AES 密钥

协议要求 **签名** 与 **加密** 使用 **两套独立的 RSA-2048 证书对**，不可混用、独立轮换（DESIGN.md §16.2 / Q34）。

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        签名链路 (Signing)                                │
├─────────────────────────────────────────────────────────────────────────┤
│  Partner 签名私钥  ──► 出站 Authorization（签 raw body 或加密后 body）   │
│  Atome  签名公钥   ◄──  入站 Webhook 验签                               │
│  （Atome 签名私钥在 Atome 侧，Partner 不接触）                            │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                        加密链路 (Encrypt)                                │
├─────────────────────────────────────────────────────────────────────────┤
│  SDK 随机 AES-256 密钥  ──► AES-ECB 加密 JSON body                      │
│  Atome  加密公钥        ──► RSA 包裹 AES 密钥 → Encrypt: header         │
│  （Atome 加密私钥在 Atome 侧，用于解包 AES 密钥）                         │
│  Partner 加密私钥（可选）◄── 解密入站加密 body（当前 credit 回调仍明文）   │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. 全部长期密钥清单（8 把 RSA + 1 把临时 AES）

### 2.1 签名对（Signing Pair）

| # | 密钥 | 持有方 | 方向 | 用途 | Partner 是否配置 SDK |
|---|------|--------|------|------|---------------------|
| S1 | **Partner 签名私钥** | 合作伙伴 | 出站 | 所有 POST/GET 的 `Authorization` 签名 | ✅ `WithPrivateKeyPEM` |
| S2 | **Partner 签名公钥** | 注册给 Atome | — | Atome 验 Partner 出站请求 | ❌ 不上 SDK，交给 Atome 运维 |
| S3 | **Atome 签名私钥** | Atome | 入站 | Atome 签 Webhook 回调 body | ❌ Partner 不持有 |
| S4 | **Atome 签名公钥** | Atome 提供给 Partner | 入站 | Partner 验 Webhook | ✅ `WithAtomePublicCertPEM` |

**算法**：RSA-2048 + SHA-256 + PKCS#1 v1.5 → Base64 → `Authorization` header

**签名内容**：
- 普通 POST：签 **明文 JSON body** 原始字节
- 加密 POST（`/credit-information`、`/credit-application`）：签 **AES 加密后的 body**（先加密，再签名）

---

### 2.2 加密对（Encrypt Pair）

| # | 密钥 | 持有方 | 方向 | 用途 | Partner 是否配置 SDK |
|---|------|--------|------|------|---------------------|
| E1 | **Partner 加密私钥** | 合作伙伴 | 入站（预留） | 解密 Atome 发来的 RSA 包裹 / 加密 body | 可选 `WithEncryptPrivateKeyPEM` |
| E2 | **Partner 加密公钥** | 注册给 Atome | — | Atome 向 Partner 发加密数据时使用 | ❌ 不上 SDK，交给 Atome 运维 |
| E3 | **Atome 加密私钥** | Atome | 出站侧解包 | Atome 解包 Partner 请求里的 AES 密钥 | ❌ Partner 不持有 |
| E4 | **Atome 加密公钥** | Atome 提供给 Partner | 出站 | Partner 用其 RSA 包裹每请求的 AES 密钥 | ✅ `WithEncryptAtomePublicCertPEM` |

**算法**：AES-256-ECB-PKCS5 加密 body；RSA-PKCS#1 v1.5 包裹 AES 密钥 → `Encrypt: symmetricKey=...` header

**仅以下接口需要 E4**：
- `POST /credit-information`
- `POST /credit-application`

---

### 2.3 临时对称密钥（每请求生成，非证书）

| 密钥 | 生成方 | 生命周期 | SDK 配置 |
|------|--------|----------|----------|
| **AES-256 密钥**（32 字节，字符集 A–Z） | SDK `encrypt.RandomAESKey()` | 单次请求；重试复用同一密钥 | 无需配置，自动随机 |

实现：`atomefin/encrypt/aes.go`、`atomefin/encrypt/envelope.go` → `encrypt.Marshal()`

---

## 3. SDK 配置项对照表

### 3.1 生产 `atomefin.Client`

| SDK Option | 对应密钥 | Client 内部字段 | 读取 API |
|------------|----------|-----------------|----------|
| `WithPrivateKeyPEM` | S1 Partner 签名私钥 | `signer` | — |
| `WithSigner` | S1（自定义 Signer 实现） | `signer` | — |
| `WithAtomePublicCertPEM` | S4 Atome 签名公钥 | `verifier` | `c.Verifier()` |
| `WithAtomePublicKey` | S4（已解析 `*rsa.PublicKey`） | `verifier` | `c.Verifier()` |
| `WithEncryptAtomePublicCertPEM` | E4 Atome 加密公钥 | `encryptAtomePub` | `c.EncryptAtomePublicKey()` |
| `WithEncryptPrivateKeyPEM` | E1 Partner 加密私钥 | `encryptPriv` | `c.EncryptPrivateKey()` |

**组合配置（v0.6+）**：

```go
atomefin.WithAtomeCerts(
    atomefin.AtomeCertSource{PartnerPriv: signPrivPEM, AtomePub: atomeSignPubPEM},   // 签名对
    atomefin.AtomeCertSource{PartnerPriv: encryptPrivPEM, AtomePub: atomeEncryptPubPEM}, // 加密对
)
```

定义：`atomefin/options.go` → `AtomeCertSource`、`WithAtomeCerts`

---

### 3.2 入站 Webhook（`atomefin/callback`）

Webhook 只涉及 **签名对 S3/S4**，不涉及加密对。

| 构造方式 | 对应密钥 | 代码 |
|----------|----------|------|
| `callback.FromCertPEMs([][]byte{pem})` | S4（可多个，证书轮换） | `atomefin/callback/verifier.go` |
| `callback.NewVerifier([]sign.Verifier{…})` | S4（底层验签器列表） | 同上 |
| `callback.FromClient(c)` | 复用 `c` 上的 `WithAtomePublicCertPEM` | 同上 |

**示例环境变量**（`examples/webhook_server/main.go`）：

| 变量 | 含义 |
|------|------|
| `ATOME_FIN_ATOME_CERT_PEM` | 单个 Atome **签名**公钥 PEM 文件路径 |
| `ATOME_FIN_ATOME_CERT_PEMS` | 多个签名公钥，冒号分隔（证书轮换窗口） |

---

### 3.3 测试 Mock（`atomefin/mock`）

| Mock Option / 函数 | 对应密钥 | 说明 |
|--------------------|----------|------|
| `WithMockKeysAllowed()` | S1 + E1 + E4（bundled） | 一次性启用内置测试密钥 |
| `WithSigningKeyPEM` | S1 | 自定义 Partner 签名私钥 |
| `WithEncryptKeyPair(atomePub, partnerPriv)` | E4 + E1 | 自定义加密密钥对 |
| `WithVerifierPubCertPEM` | S4 | Mock Client 上的验签公钥 |
| `MockSigningPrivKeyPEM()` | S1 测试私钥 | `atomefin/mock/keys.go` |
| `MockSigningPubCertPEM()` | S4 测试公钥 | 同上 |
| `MockEncryptPrivKeyPEM()` | E1 测试私钥 | 同上 |
| `MockEncryptPubCertPEM()` | E4 测试公钥 | 同上 |
| `WithFireSignerKeyPEM` | 模拟 Atome 签回调用的私钥 | `atomefin/mock/callback.go` |
| `WithIdempotencyDecryptKey` | E1 | Mock Server 解密加密 POST 做幂等缓存 |
| `WithResponseSigning` | Partner 测试私钥 | Mock Server 给响应加 Authorization |

内置 PEM 文件：`atomefin/mock/testdata/mock_signing_{priv,pub}.pem`、`mock_encrypt_{priv,pub}.pem`

---

## 4. 按业务场景：需要配哪些密钥

| 场景 | S1 签名私钥 | S4 签名公钥 | E4 加密公钥 | E1 加密私钥 |
|------|:-----------:|:-----------:|:-----------:|:-----------:|
| Auth / Capture / Refund / 账单 / 交易等普通出站 | ✅ | — | — | — |
| 接收 Webhook（Auth/Capture/Refund 回调等） | — | ✅ | — | — |
| `/credit-information` / `/credit-application` | ✅ | — | ✅ | 可选 |
| 出站 + 回调一体（单 Client + callback Handler） | ✅ | ✅ | 按需 | 可选 |
| 仅 Mock 单元测试 | ✅（或 `WithMockKeysAllowed`） | 按需 | Credit 测试时需要 | 按需 |

---

## 5. 密钥在代码中的流转

### 5.1 出站普通签名

```
WithPrivateKeyPEM(S1)
       │
       ▼
Client.DoSigned / payment.Auth …
       │
       ├─ MarshalSigning(body)
       ├─ signer.Sign(body)  ──► Authorization header
       └─ HTTP POST
```

代码：`atomefin/doer.go` → `signAndDispatch`

### 5.2 出站 Credit 混合加密 + 签名

```
WithPrivateKeyPEM(S1) + WithEncryptAtomePublicCertPEM(E4)
       │
       ▼
Client.DoEncryptedSigned
       │
       ├─ encrypt.Marshal(plain, E4)
       │     ├─ RandomAESKey() → AES 加密 body
       │     └─ RSA wrap AES key → Encrypt: header
       ├─ signer.Sign(encryptedBody) → Authorization
       └─ HTTP POST
```

代码：`atomefin/doer.go`、`atomefin/encrypt/`、`atomefin/credit/credit.go`

### 5.3 入站 Webhook 验签

```
FromCertPEMs(S4)
       │
       ▼
callback.AuthHandler(verifier, fn)
       │
       ├─ 读 raw body（限 1 MiB）
       ├─ verifier.Verify(body, Authorization)  ← 用 S4
       ├─ JSON 解码
       └─ 业务 fn → AckResponse
```

代码：`atomefin/callback/handler.go`、`atomefin/callback/verifier.go`

---

## 6. PEM 加载与格式

| 函数 | 文件 | 支持格式 |
|------|------|----------|
| `sign.LoadPrivateKeyPEM` | `atomefin/sign/pem.go` | PKCS#1 / PKCS#8 私钥 |
| `sign.LoadPublicCertPEM` | `atomefin/sign/pem.go` | CERTIFICATE / PUBLIC KEY / RSA PUBLIC KEY |

约束：RSA 模长 ≥ **2048 bit**；加密 PEM 带密码暂不支持。

---

## 7. 测试 / 向量用 PEM 文件

| 路径 | 角色 |
|------|------|
| `atomefin/sign/testdata/external_pub.pem` | 外部 openssl 向量 — 签名公钥 |
| `atomefin/sign/testdata/external_priv.pem` | 外部 openssl 向量 — 签名私钥 |
| `atomefin/encrypt/testdata/encrypt_atome_pub.pem` | E4 测试公钥 |
| `atomefin/encrypt/testdata/encrypt_partner_priv.pem` | E1 测试私钥 |
| `atomefin/mock/testdata/mock_signing_*.pem` | Mock 签名对 |
| `atomefin/mock/testdata/mock_encrypt_*.pem` | Mock 加密对 |

---

## 8. 环境变量速查（示例程序）

| 变量 | 密钥 | 示例 |
|------|------|------|
| `ATOME_FIN_PRIV_KEY_PEM` | S1 Partner 签名私钥路径 | `examples/auth_capture` |
| `ATOME_FIN_ATOME_CERT_PEM` | S4 Atome 签名公钥路径 | `examples/webhook_server` |
| `ATOME_FIN_ATOME_CERT_PEMS` | S4 多证书（轮换） | `examples/webhook_server` |

Credit 加密公钥（E4）在示例中无独立环境变量，需在代码里 `WithEncryptAtomePublicCertPEM` 加载。

---

## 9. 与 Atome 运维交换清单

**Partner → Atome 登记（公钥，不进 SDK 私钥配置）**

| 交付物 | 对应编号 |
|--------|----------|
| Partner 签名公钥（由 S1 导出） | S2 |
| Partner 加密公钥（由 E1 导出，若协议要求双向加密证书交换） | E2 |

**Atome → Partner 提供（写入 SDK）**

| 交付物 | SDK 配置 | 对应编号 |
|--------|----------|----------|
| Atome 签名公钥 PEM | `WithAtomePublicCertPEM` / Webhook `FromCertPEMs` | S4 |
| Atome 加密公钥 PEM | `WithEncryptAtomePublicCertPEM` | E4 |

**Partner 本地保管（私钥，绝不上传）**

| 交付物 | SDK 配置 | 对应编号 |
|--------|----------|----------|
| Partner 签名私钥 PEM | `WithPrivateKeyPEM` | S1 |
| Partner 加密私钥 PEM（可选） | `WithEncryptPrivateKeyPEM` | E1 |

---

## 10. 常见混淆点

| 错误 | 正确做法 |
|------|----------|
| 把 Atome **签名**公钥配到 `WithEncryptAtomePublicCertPEM` | 加密公钥单独交付，用 E4 专用 Option |
| 把 Partner **签名**私钥当加密私钥用 | S1 与 E1 是不同证书对 |
| Credit 接口只配签名私钥 | 必须同时配 E4，否则本地报 `ValidationError` |
| Webhook 验签失败 | 确认用的是 S4，且验的是 **raw body**，不是解析后的 JSON |
| 加密 POST 签名验不过 | Authorization 签的是 **加密后 body**，不是明文 |

---

## 11. 源码索引

| 主题 | 文件 |
|------|------|
| 全部 `With*` 选项 | `atomefin/options.go` |
| Client 密钥字段 / 读取器 | `atomefin/client.go` |
| 出站签名 / 加密分发 | `atomefin/doer.go` |
| RSA 签名 / 验签 | `atomefin/sign/signer.go`、`verifier.go`、`pem.go` |
| AES + RSA 混合加密 | `atomefin/encrypt/` |
| Webhook 验签 | `atomefin/callback/verifier.go`、`handler.go` |
| Credit 加密调用 | `atomefin/credit/credit.go` |
| Mock 测试密钥 | `atomefin/mock/keys.go` |
| Webhook 示例 | `examples/webhook_server/main.go` |
| Auth 出站示例 | `examples/auth_capture/main.go` |
