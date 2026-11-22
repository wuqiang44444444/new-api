---
status: current
owner: Dev Team
last-reviewed: 2026-11-22
---

# Azure 跨租户 PostgreSQL 私网配置手册

## 适用范围

本手册用于在网关 VM 的 Azure VNet 建立 Private Endpoint，跨区域、跨 Entra 租户访问 Azure Database for PostgreSQL
Flexible Server。目标是只允许网关 VM 发起 PostgreSQL 连接，同时保留网关与代理 VM 之间已有的 VNet Peering 和应用流量。
现状与验收记录见 [Azure 网关环境与访问边界](Azure网关环境与访问边界.md)。

数据库必须使用支持 Private Endpoint 的连接模式。若数据库已经部署，在 **Networking** 核对连接模式和公网开关，
然后从第 1 步开始。使用委派子网的 VNet Integration 环境应按其网络设计评估连接方式。VNet Peering 提供两侧网络的
私网路由，单独建立 Peering 无法为当前模式的 PostgreSQL 提供私网入口。
[PostgreSQL 网络模式](https://learn.microsoft.com/en-us/azure/postgresql/network/how-to-networking)

若从零新建，在**数据库订阅的 Azure Portal** 创建 Azure Database for PostgreSQL Flexible Server。

1. 在 **Basics** 选择数据库资源组、区域、实例名和 PostgreSQL 主版本。根据容量和可用性目标选择计算规格、存储、
   HA 模式、备份保留期和备份冗余。区域内 HA 处理可用区故障，异地备份用于跨区域恢复，两项分别选择。
2. 在 **Networking → Connectivity method** 选择 **Public access (allowed IP addresses) and private endpoints**。
   要求从创建起仅走私网时，取消勾选下方独立的 **Allow public access to this resource through the internet using a public IP address**。
   服务器建成后等待端点批准才具备该私网入口。
3. 在 **Security** 按部署要求选择加密密钥。本环境使用 Azure 托管密钥。配置所需的管理员认证，
   将数据库凭据存入批准的秘密存储。在 **Review + create** 核对后部署，等状态为 `Ready`。
   [Azure Portal 创建 PostgreSQL Flexible Server](https://learn.microsoft.com/en-us/azure/postgresql/flexible-server/quickstart-create-server-portal)

## 1. 管理终端准备与资源核对

**执行位置**为管理员工作站的同一个 Bash 或 zsh 会话。每条管理命令都显式指定目标订阅，资源 ID 命令从 ID 读取订阅。
需要 VM 侧网络资源的创建和更新权限，以及数据库侧私有端点批准及网络设置权限。

```bash
command -v az
az version
az login
az account list --query "[?state=='Enabled'].{Name:name,Default:isDefault}" -o table
```

macOS 上若 Homebrew 已安装 CLI 而 `az` 找不到，先运行 `/opt/homebrew/bin/az version` 验证安装，
再检查终端是否加载 Homebrew 路径。Apple Silicon Homebrew 默认路径可以用 `eval "$(/opt/homebrew/bin/brew shellenv)"`
加载到当前会话。已有有效 Azure 登录时可跳过 `az login`。若只有租户级条目或出现 `SubscriptionNotFound`，
先切换到有目标订阅权限的账号或目录。运行命令块时，最后一行无需续行反斜杠。

将下列变量替换为 Azure 中的实际值。`NET_*` 为私有端点所在网络组，通常是 VM VNet 所在的资源组。
`PE_REGION` 取 VNet 区域，`DB_SERVER` 取 Flexible Server 资源名。

```bash
VM_SUB='<VM 订阅名或 ID>'
VM_RG='<VM 资源组>'
VM_NAME='<网关 VM 名>'
NET_RG='<VNet 资源组>'
VM_VNET='<网关 VNet 名>'
VM_SUBNET='<计划放置端点的子网名>'
PE_REGION='<VNet 区域代码>'
DB_SUB='<数据库订阅名或 ID>'
DB_RG='<数据库资源组>'
DB_SERVER='<Flexible Server 资源名>'
PE_NAME='<私有端点名>'
NSG_NAME='<端点访问控制 NSG 名>'
DNS_ZONE='privatelink.postgres.database.azure.com'
DNS_LINK='<私有 DNS 的 VNet 链接名>'

az vm show --subscription "$VM_SUB" -g "$VM_RG" -n "$VM_NAME" \
  --query '{region:location,nics:networkProfile.networkInterfaces[].{id:id,primary:primary}}' -o json
az network vnet subnet show --subscription "$VM_SUB" -g "$NET_RG" \
  --vnet-name "$VM_VNET" -n "$VM_SUBNET" \
  --query '{id:id,nsg:networkSecurityGroup.id,policy:privateEndpointNetworkPolicies,routeTable:routeTable.id}' -o json
az postgres flexible-server show --subscription "$DB_SUB" -g "$DB_RG" -n "$DB_SERVER" \
  --query '{state:state,region:location,host:fullyQualifiedDomainName,public:network.publicNetworkAccess,delegatedSubnet:network.delegatedSubnetResourceId}' -o json
az postgres flexible-server private-endpoint-connection list --subscription "$DB_SUB" \
  -g "$DB_RG" -s "$DB_SERVER" \
  --query '[].{name:name,status:privateLinkServiceConnectionState.status,endpoint:privateEndpoint.id}' -o table
```

确认子网、数据库、订阅和区域都正确，数据库为 `Ready`。VM 只有一张 NIC 时使用列表中的唯一项，即使 `primary` 为空。
有多张 NIC 时，结合 VM 到端点的实际路由确定源 NIC。已存在的端点和同名 DNS Zone 优先核对后复用。
如果公网曾启用，在关闭前用 `az postgres flexible-server firewall-rule list` 核对现有规则及依赖。
对正在服务的网关，从两侧分别用 `az network vnet peering list` 核对连接状态，并按第 6 步采样代理到网关的健康检查，
作为变更前基线。

## 2. 两个订阅注册资源提供程序

**执行位置**仍为管理终端。两个订阅均需要 `Microsoft.Network` 和目标服务提供程序 `Microsoft.DBforPostgreSQL`。
用 `az provider show --subscription "$VM_SUB" --namespace Microsoft.DBforPostgreSQL --query registrationState -o tsv`
核对各组合，只对 `NotRegistered` 的项执行 `az provider register`。本环境需要补齐的组合如下。
[跨订阅端点注册与批准](https://learn.microsoft.com/en-us/azure/private-link/how-to-approve-private-link-cross-subscription)

```bash
az provider register --subscription "$VM_SUB" --namespace Microsoft.DBforPostgreSQL --wait
az provider register --subscription "$DB_SUB" --namespace Microsoft.Network --wait
```

## 3. VM 侧创建端点，数据库侧批准

先在数据库订阅读取资源 ID，再在 VM 订阅发起手动请求。端点必须和目标 VNet 位于同一个区域。
目标数据库可以在另一个区域，子资源固定选 `postgresqlServer`。
[Azure 创建 PostgreSQL 私有端点](https://learn.microsoft.com/en-us/azure/postgresql/network/how-to-networking-servers-deployed-public-access-add-private-endpoint)

```bash
DB_ID=$(az postgres flexible-server show --subscription "$DB_SUB" -g "$DB_RG" -n "$DB_SERVER" \
  --query id -o tsv)

if [[ -n "$DB_ID" ]]; then
  az network private-endpoint create --subscription "$VM_SUB" -g "$NET_RG" -n "$PE_NAME" \
    --location "$PE_REGION" --vnet-name "$VM_VNET" --subnet "$VM_SUBNET" \
    --private-connection-resource-id "$DB_ID" --group-id postgresqlServer \
    --connection-name gateway-to-postgresql --manual-request true \
    --request-message 'PostgreSQL private access from the gateway VM'
fi
```

数据库订阅的连接列表应出现 `Pending`。从 `Endpoint` 列核对完整资源 ID，把同一行 `Name` 填入批准命令。
VM 侧请求名称和数据库侧连接名称由各自的资源管理接口返回。审批后的连接状态应为 `Approved`。
[Azure 批准 PostgreSQL 私有端点](https://learn.microsoft.com/en-us/azure/postgresql/network/how-to-networking-servers-deployed-public-access-approve-private-endpoint)

```bash
az postgres flexible-server private-endpoint-connection list --subscription "$DB_SUB" \
  -g "$DB_RG" -s "$DB_SERVER" \
  --query '[].{Name:name,Status:privateLinkServiceConnectionState.status,Endpoint:privateEndpoint.id}' -o table

CONNECTION_NAME='<数据库侧列表中该端点对应的 Name>'
az postgres flexible-server private-endpoint-connection approve --subscription "$DB_SUB" \
  -g "$DB_RG" -s "$DB_SERVER" -n "$CONNECTION_NAME" --description 'Approved for the gateway VM'

az network private-endpoint show --subscription "$VM_SUB" -g "$NET_RG" -n "$PE_NAME" \
  --query '{resource:provisioningState,connection:manualPrivateLinkServiceConnections[0].privateLinkServiceConnectionState.status}' \
  -o json
```

## 4. VM 侧配置私有 DNS

在 VM 订阅创建或复用 `privatelink.postgres.database.azure.com`。DNS Zone Group 负责端点 A 记录，
VNet Link 让网关 VNet 使用这条记录。已有同名 Zone 位于另一资源组时，用该资源组与 Zone 的完整 ID。
本环境使用 Azure 默认 DNS。使用自定义 DNS 的环境还需配置正确的解析路径。
[PostgreSQL Private Link DNS](https://learn.microsoft.com/en-us/azure/postgresql/network/concepts-networking-private-link#private-link-and-dns)

```bash
DNS_RG="$NET_RG"
az network private-dns zone create --subscription "$VM_SUB" -g "$DNS_RG" -n "$DNS_ZONE"
ZONE_ID=$(az network private-dns zone show --subscription "$VM_SUB" -g "$DNS_RG" -n "$DNS_ZONE" \
  --query id -o tsv)
VNET_ID=$(az network vnet show --subscription "$VM_SUB" -g "$NET_RG" -n "$VM_VNET" \
  --query id -o tsv)

az network private-endpoint dns-zone-group create --subscription "$VM_SUB" -g "$NET_RG" \
  --endpoint-name "$PE_NAME" -n default --private-dns-zone "$ZONE_ID" --zone-name postgresql
az network private-dns link vnet create --subscription "$VM_SUB" -g "$DNS_RG" -z "$DNS_ZONE" \
  -n "$DNS_LINK" --virtual-network "$VNET_ID" --registration-enabled false

az network private-dns link vnet show --subscription "$VM_SUB" -g "$DNS_RG" -z "$DNS_ZONE" \
  -n "$DNS_LINK" --query '{state:provisioningState,link:virtualNetworkLinkState}' -o json
az network private-dns record-set a show --subscription "$VM_SUB" -g "$DNS_RG" -z "$DNS_ZONE" -n "$DB_SERVER" \
  --query 'aRecords[].ipv4Address' -o tsv
```

等 VNet Link 为 `Completed`。从网关 VM 解析数据库原始 FQDN，结果必须等于端点 NIC 的私有 IP。
应用和 SQL 客户端使用原始 FQDN，以便 TLS 校验服务器名称。

## 5. 网关 VM 独占端点与公网关闭

共享子网上新建并绑定 NSG 会引入 Azure 默认规则。当前方案先把完整规则建在**未关联**的 NSG 上，再一次性关联子网
并启用 `NetworkSecurityGroupEnabled`。只对端点地址增加限制。其它入站和出站继续由现有 NIC NSG 决策。
保留两侧 Peering、VM 网卡、路由表和代理配置。子网已经关联 NSG 或包含其它端点时，先审查已有规则与影响范围，
下列新建流程适用于原子网无 NSG 的情形。
[Azure 私有端点网络策略](https://learn.microsoft.com/en-us/azure/private-link/disable-private-endpoint-network-policy) ·
[Azure 子网与网卡 NSG 的处理顺序](https://learn.microsoft.com/en-us/azure/virtual-network/network-security-group-how-it-works)

| 优先级 | 方向 | 源 | 目标 | 协议与端口 | 动作 |
| --- | --- | --- | --- | --- | --- |
| `100` | 入站 | 网关 VM 私有 IP `/32` | 私有端点 IP `/32` | TCP `5432` | Allow |
| `110` | 入站 | 任意 | 私有端点 IP `/32` | 任意 | Deny |
| `4000` | 入站 | 任意 | 任意 | 任意 | Allow，保留原有子网层行为 |
| `4000` | 出站 | 任意 | 任意 | 任意 | Allow，保留原有子网层行为 |

先从 Azure 实时读取两张 NIC 的地址，再在未关联的新 NSG 上一次性创建四条规则。运行前确认 NSG 名尚未使用。
`VM_NIC_ID` 取第 1 步确认的网关主网卡 ID。用 NIC 地址限制来源时，VM 私有 IP 或端点 IP 变更后必须重新核对规则。

```bash
VM_NIC_ID='<第 1 步确认的主 NIC ID>'
VM_IP=$(az network nic show --ids "$VM_NIC_ID" --query 'ipConfigurations[?primary].privateIPAddress | [0]' -o tsv)
PE_NIC_ID=$(az network private-endpoint show --subscription "$VM_SUB" -g "$NET_RG" -n "$PE_NAME" \
  --query 'networkInterfaces[0].id' -o tsv)
PE_IP=$(az network nic show --ids "$PE_NIC_ID" --query 'ipConfigurations[0].privateIPAddress' -o tsv)
VM_SUB_ID=$(az account show --subscription "$VM_SUB" --query id -o tsv)
NSG_ID="/subscriptions/$VM_SUB_ID/resourceGroups/$NET_RG/providers/Microsoft.Network/networkSecurityGroups/$NSG_NAME"

NSG_BODY=$(python3 - "$PE_REGION" "$VM_IP" "$PE_IP" <<'PY'
import ipaddress
import json
import sys

region, vm, endpoint = sys.argv[1:]
vm = f"{ipaddress.IPv4Address(vm)}/32"
endpoint = f"{ipaddress.IPv4Address(endpoint)}/32"
assert vm != endpoint

def rule(name, priority, direction, access, source, destination, protocol="*", port="*"):
    return {"name": name, "properties": {
        "priority": priority, "direction": direction, "access": access,
        "sourceAddressPrefix": source, "destinationAddressPrefix": destination,
        "protocol": protocol, "sourcePortRange": "*", "destinationPortRange": port,
    }}

print(json.dumps({"location": region, "properties": {"securityRules": [
    rule("allow-tokenvm-postgresql", 100, "Inbound", "Allow", vm, endpoint, "Tcp", "5432"),
    rule("deny-other-private-endpoint-ingress", 110, "Inbound", "Deny", "*", endpoint),
    rule("preserve-existing-subnet-ingress", 4000, "Inbound", "Allow", "*", "*"),
    rule("preserve-existing-subnet-egress", 4000, "Outbound", "Allow", "*", "*"),
]}}))
PY
)

if [[ -n "$NSG_BODY" ]]; then
  az rest --method put --subscription "$VM_SUB" \
    --url "https://management.azure.com$NSG_ID?api-version=2026-03-01" \
    --headers 'If-None-Match=*' 'Content-Type=application/json' --body "$NSG_BODY" \
    --query '{state:properties.provisioningState,rules:length(properties.securityRules)}' -o json
fi
az network nsg show --subscription "$VM_SUB" -g "$NET_RG" -n "$NSG_NAME" \
  --query '{state:provisioningState,subnets:subnets[].id,rules:securityRules[].{name:name,priority:priority,access:access,state:provisioningState}}' \
  -o json
```

确认 NSG 和四条规则均为 `Succeeded`、NSG 尚未关联。提前从代理 VM 持续采样网关 `GET /api/status`，
核对变更窗口内的 HTTP 状态。再次确认子网尚无 NSG 后，在 **VM 订阅**执行关联。
[Azure NSG 创建 API](https://learn.microsoft.com/en-us/rest/api/virtualnetwork/network-security-groups/create-or-update?view=rest-virtualnetwork-2026-03-01)

```bash
az network vnet subnet update --subscription "$VM_SUB" -g "$NET_RG" \
  --vnet-name "$VM_VNET" -n "$VM_SUBNET" --network-security-group "$NSG_ID" \
  --private-endpoint-network-policies NetworkSecurityGroupEnabled \
  --query '{state:provisioningState,nsg:networkSecurityGroup.id,policy:privateEndpointNetworkPolicies}' -o json
```

确认网关 VM 能通过私网完成 PostgreSQL TLS 握手，代理到网关的健康检查仍成功。随后在 **数据库订阅**
关闭公网。如果已经是 `Disabled`，直接核验状态即可。
[Azure PostgreSQL 公网开关](https://learn.microsoft.com/en-us/azure/postgresql/network/how-to-networking-servers-deployed-public-access-enable-disable-public-access)

```bash
az postgres flexible-server update --subscription "$DB_SUB" -g "$DB_RG" -n "$DB_SERVER" \
  --public-access Disabled --query '{state:state,public:network.publicNetworkAccess}' -o json
```

完成条件为数据库 `Ready`、公网 `Disabled`、私有端点 `Approved`，以及下面各项数据面验收通过。
公网关停后，CLI 的防火墙规则操作可能不可用，此时以 `network.publicNetworkAccess` 为准。

## 6. 分位置验收

| 执行位置 | 检查 | 成功信号 |
| --- | --- | --- |
| 管理终端，数据库订阅 | 读 PostgreSQL 状态和私有端点连接列表 | `Ready`、公网 `Disabled`，只有预期端点获批准 |
| 网关 VM 或实际应用运行环境 | 解析 PostgreSQL 原始 FQDN，校验 TLS 并执行 SQL | IP 等于端点 NIC 地址，证书校验通过，`SELECT 1` 返回 `1` |
| 对等代理 VM | 直接连接端点 IP 的 TCP `5432` | 新连接超时或被拒绝 |
| 网关 VM 与代理 VM | 重测原有对等业务路径及健康检查 | 代理到网关 `GET /api/status` 返回 HTTP `200`，原先可达的端口继续可达 |

**在网关 VM 的 SSH 或 Azure Bastion 终端**，设置 `PG_HOST` 为 Azure 返回的原始 FQDN，`PG_USER` 为授权测试用户，
`PG_DB` 为要验证的数据库，`PG_CA_BUNDLE` 为该 VM 上含受信根 CA 的 PEM 文件。测试凭据通过交互提示、受保护的
密码文件或经授权的秘密存储提供。Azure VM Run Command 适合执行无秘密的只读探测。
可用 `python3 -c 'import ssl; print(ssl.get_default_verify_paths().cafile or "")'` 查找 VM 的默认 CA 文件，
并用下面的 TLS 命令验证它是否信任该服务器。

```bash
getent ahostsv4 "$PG_HOST"
timeout 10 openssl s_client -starttls postgres -connect "$PG_HOST:5432" \
  -servername "$PG_HOST" -verify_hostname "$PG_HOST" -verify_return_error \
  -CAfile "$PG_CA_BUNDLE" </dev/null

psql -X -W \
  "host=$PG_HOST port=5432 dbname=$PG_DB user=$PG_USER sslmode=verify-full sslrootcert=$PG_CA_BUNDLE connect_timeout=5" \
  -c 'SELECT current_user, current_database(), 1 AS ok;'
```

VM 没有 `psql` 时，可在 **Linux 网关 VM** 用一次性客户端容器，挂载已在 VM 上验证的 CA 文件以校验证书。
此命令放在 VM 的终端运行。
[Azure PostgreSQL TLS 配置](https://learn.microsoft.com/en-us/azure/postgresql/security/security-tls-how-to-connect)

```bash
docker run --rm -it --network host \
  --mount "type=bind,src=$PG_CA_BUNDLE,dst=/probe-ca.pem,readonly" \
  postgres:18 psql -X -W \
  "host=$PG_HOST port=5432 dbname=$PG_DB user=$PG_USER sslmode=verify-full sslrootcert=/probe-ca.pem connect_timeout=5" \
  -c 'SELECT current_user, current_database(), 1 AS ok;'
```

在 **代理 VM** 设置 `PE_IP`、`VM_IP` 和 `GATEWAY_PORT` 为现场核对过的值，再分别测试数据库和业务路径。
端点负向测试使用端点 IP，避免把 DNS 解析差异误判成 NSG 拒绝。

```bash
timeout 6 bash -c 'true > /dev/tcp/$1/5432' _ "$PE_IP"
curl --fail --silent --show-error -o /dev/null -w '%{http_code}\n' \
  "http://$VM_IP:$GATEWAY_PORT/api/status"
```

按同一连接执行多次 `SELECT 1` 并记录样本量和分位数时，在 `psql` 中使用 `\timing on`。TCP 建连、TLS 握手及
已认证连接上的 SQL 查询是不同测量，登记时写明口径。NSG 规则针对新连接，收紧前已存在的数据库会话需单独核对。
[Azure NSG 状态与现有连接](https://learn.microsoft.com/en-us/azure/virtual-network/network-security-groups-overview)

## 7. 故障处置与回滚

| 现象 | 处置 |
| --- | --- |
| 管理终端 `SubscriptionNotFound` | 核对登录租户、实际订阅和 RBAC 权限。 |
| 资源 `Succeeded`，连接仍为 `Pending` | 在数据库订阅按对应端点资源 ID 批准连接，等待两边显示 `Approved`。 |
| 网关 VM 域名解析异常 | 核对 Zone Group、A 记录、VNet Link 与 VM 实际 DNS。 |
| Mac 或其它外部机器连接超时 | 把测试移到网关 VM。Docker 的 `--network host` 使用容器宿主网络，外部电脑仍没有网关 VM 的私网来源。`psql -W` 会在连接前提示输入密码。 |
| TLS 证书校验失败 | 检查原始 FQDN、受信根 CA 及容器证书库。继续使用 `verify-full`，从已验证的 VM CA 文件重试。 |
| 代理也能新建 PostgreSQL 连接 | 核对 `NetworkSecurityGroupEnabled`、实际关联 NSG、端点和源地址，以及规则顺序。 |
| 代理到网关的业务检查退化 | 优先保持业务路径，核对变更窗口与子网 NSG 命中规则。按变更前状态评估恢复子网关联，恢复后重新测业务和数据库。 |

如果变更前子网无 NSG 且私有端点策略为 `Disabled`，需要紧急恢复原子网策略时，可在 **VM 订阅**执行下面的子网操作。
回退会撤销端点的来源限制，应按事件处置流程限时执行并记录影响。数据库公网开关独立管理。

```bash
az network vnet subnet update --subscription "$VM_SUB" -g "$NET_RG" \
  --vnet-name "$VM_VNET" -n "$VM_SUBNET" --network-security-group null \
  --private-endpoint-network-policies Disabled \
  --query '{state:provisioningState,nsg:networkSecurityGroup.id,policy:privateEndpointNetworkPolicies}' -o json
```

回退后核对 `nsg` 为空、策略为 `Disabled`，并复测两台 VM 的既有业务路径。若原子网已有 NSG 或其它网络策略，
应按变更前记录恢复原关联。数据库迁移、应用切库及相应的回退另见 [SQLite 全量迁移 PostgreSQL 手册](SQLite全量迁移PostgreSQL手册.md)。
