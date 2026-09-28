---
status: superseded
owner: Dev Team
last-reviewed: 2026-09-28
superseded-by: Vidu两站差异与现有渠道适配结论.md
---

# Vidu 国内站初次调研来源更正

用户已更正本次接入文档。此前普通 Vidu `/ent/v2` 与 `Token` 的调查不适用于本次 Drama/Seedance
线路；原“必须新增 Vidu 南向协议”的判断撤回，不再作为接入依据。

- 正确国内来源：[飞书 Drama 国内文档](https://shengshu.feishu.cn/wiki/PkFHwrme8ihMA8kshbgc5QwPnxe)。
- 正确国外来源：[飞书 Drama 海外文档](https://shengshu.feishu.cn/wiki/UdZNw56HxiLfEykYOjmcsaYQn08)。
- [国内原始调用信息](Vidu国内站视频接口原始信息.md)。
- [国外原始调用信息](Vidu国外站视频接口原始信息.md)。
- [重新核对后的 Seedance 渠道支持结论](Vidu两站差异与现有渠道适配结论.md)。

当前结论以真实测试为准：ModelArk V3 创建可复用，但 Vidu 返回的字符串数字及用量不能由现有官方协议直接解析，需 Vidu 响应合同适配后重新验收。详见上述支持结论及其真实测试记录。
