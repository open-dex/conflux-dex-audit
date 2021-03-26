# CONFLUX DEX核账服务
该项目为**Conflux DEX**提供核账服务，包括实时和定时核账监控，旨在保证用户资金安全。

# 1. 跨链资产
```
conflux-dex-audit shuttleflow all --matchflow https://api.matchflow.io
```
> 注意：跨链资产的核账程序只适用于实盘，所以所有包含跨链资产的命令均需添加`--matchflow https://api.matchflow.io`

| Flag | Description |
| -- | -- |
| --btcinit | BTC转账监听的初始区块高度 |
| --ethinit | ETH转账及USDT合约事件监听的初始区块数 |
| --ethdelay | ETH转账及USDT合约事件监听的延迟区块数 |
| --ethurl | ETH API 网址 |
| --leveldb | leveldb路径 |
| --sync | 用于手动同步ETH链内钱包资产 |
| --timeoutM | 超时时长（以分钟为单位） |
| --timeoutS | 超时时长（以秒钟为单位） |

## 1.1 操作预警
- 不同链根据Timeout实时监测：充值/提现请求是否处理完，超时报警
### 1.1.1 充值
```
conflux-dex-audit shuttleflow deposit --matchflow https://api.matchflow.io
```
| Flag | Description |
| -- | -- |
| --sync | 用于手动同步ETH链内钱包资产 |
| --ethinit | ETH转账及USDT合约事件监听的初始区块 |

## 1.2 余额预警
- 周期性监测：BTC >= cBTC, ETH >= cETH, USDT >= cUSDT
```
conflux-dex-audit shuttleflow balance --matchflow https://api.matchflow.io
```

# 2. 链内资产
## 2.1 周期余额预警
- 核账程序启动时检查；
- 每小时检查一次，或者每N个Epoch检查一次；
- 检查包括以下3项，且各项余额都应该一致：
    1. CRCL合约账户在ERC777合约中持有的资产余额；
    2. CRCL合约的total supply；
    3. CRCL合约所有账户余额的总和（通过数据迁移接口获取用户列表）；
## 2.2 实时余额预警
- 基于周期余额检查的结果，对每个Epoch的event log进行检查，保证Epoch级别的预警；
- 总余额一致性检查：
    1. CRCL合约账户在ERC777合约中持有的资产余额；
    2. CRCL合约的total supply；
- 基于前一个Epoch的用户余额列表，通过Event logs更新当前Epoch的用户余额，并且检查更新后的账户余额与链上是否一致（只检查有余额更新的账户）；
## 2.3 命令行工具（子命令）
- conflux-dex-audit boomflow：启动Boomflow核账服务；
- conflux-dex-audit boomflow balance：对于指定epoch和asset进行账户余额核账；
- conflux-dex-audit boomflow event：查看指定epoch的Event Logs所产生的账户余额变化；

# 3. 链上链下同步
- 实时余额预警（Epoch级别）：线上以Epoch单位实时监听Event计算余额
MerkleRoot，线下同样以Epoch为单位实时Replay对比计算余额MerkleRoot，出错能定位到Tx
- 周期余额预警（小时级别）：线上以Epoch单位获取当前state所有用户余额，线下以Epoch为单位实时Replay，一一对比用户余额
