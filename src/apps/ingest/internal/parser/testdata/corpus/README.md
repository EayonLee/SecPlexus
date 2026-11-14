# 解析语料库

这里的每条样本都是一条**真实格式**的报文，用于回归测试与模糊测试种子。

## 为什么单独放语料

解析器的失败几乎都来自"某个厂商的某种写法"：少一个空格、多一层引号、
转义符没配对、字段名大小写不同。只靠手写单元测试覆盖不到这些形态，
而线上一条畸形日志就能让规则静默错位。把语料沉淀成文件后：

- 每次改动解析器都会跑全部语料（`corpus_test.go`），形态回归立刻暴露；
- 同一批样本同时是 Go 原生模糊测试的种子（`fuzz_test.go`），
  变异库会在此基础上生成更恶劣的输入；
- 新增厂商时只需要往这里加一个文件，测试自动覆盖。

## 目录约定

```
corpus/<mode>/<name>.log     每条样本；<mode> 与解析模式同名（json/cef/kv/regex/delimiter）
```

## 来源与许可

语料只收录**格式规范里公开的写法**与**按公开规范自行构造**的样本，
不复制任何厂商文档原文。若要从开源数据集导样本（见下），必须在该文件顶部
注明来源、许可与获取日期。

可用的公开数据集（按需导入，注意各自许可）：

| 数据集 | 内容 | 链接 |
|---|---|---|
| Loghub | 真实系统日志（HDFS/Linux/Apache/OpenSSH/Zookeeper/Windows 等） | https://github.com/logpai/loghub |
| Azure Sentinel Sample Data | SIEM 场景样本（ASIM 模式） | https://github.com/Azure/Azure-Sentinel/tree/master/Sample%20Data |
| OCSF schema events | OCSF 事件的标准形状，用于校验我们产出的信封 | https://github.com/ocsf/ocsf-schema/tree/main/events |
| panw-fw-samples | PAN-OS 防火墙样本（合成） | https://github.com/daveherrald/panw-fw-samples |
| log-zoo | 多厂商原始日志合集 | https://github.com/sva-s1/log-zoo |

导入时用脚本裁剪成小样本（单条 ≤ 4 KiB），避免把数 GB 数据集带进仓库。

## 新增样本的要求

1. 必须是该格式**合法**的报文（能解析成功）；
2. 覆盖一种真实写法差异（不同时间格式、缺失字段、转义、大小写）；
3. 单文件一行一条，便于按行读取。
