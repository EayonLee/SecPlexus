# 接入来源与规则清单

控制台保留「数据接入」和「解析规则」两个入口。规则清单直接占据正文，可按名称、描述、厂商、产品搜索，并按来源、解析类型和事件类型过滤。移除分类树、设备类型编辑和按目标字段检索。已有规则的字段映射、样本验证、复制和导入导出继续可用。

厂商（vendor）与产品（product）是独立的来源属性。规则适配的产品和接入点实际采集的产品可以不同。两者分别保存，编辑其中一项可以保留另一项。迁移一次性删除旧接入点及自定义规则。安装时提供的 FortiGate 模板保留，厂商为 Fortinet，产品为 FortiGate。新建时可从所选规则带入建议值，操作员可覆盖。事件需要区分实际接入来源与 OCSF 产出软件。

接入详情把收包证据、协议与端点、绑定规则和来源信息放在同一底色上。操作员可从接入详情进入绑定的规则，端点缺失明确标示未配置并隐藏复制。编辑时专注可修改的内容，取消恢复持久化值。宽屏并排，窄屏依照任务顺序重排。

依据：[Graylog 接入诊断](https://go2docs.graylog.org/current/getting_in_log_data/input_diagnosis.htm)区分管理状态与实际收包；[Elastic 的 FortiGate 集成](https://github.com/elastic/integrations/blob/main/packages/fortinet_fortigate/manifest.yml)将产品和传输通道分别配置；[Wazuh 规则验证](https://documentation.wazuh.com/current/user-manual/ruleset/testing.html)用实际样本核对解码结果。视觉沿用本项目 shadcn 表格、字段组、标题与分隔线。
