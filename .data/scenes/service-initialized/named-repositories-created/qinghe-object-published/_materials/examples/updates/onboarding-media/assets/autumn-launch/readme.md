---
entity: example/autumn-launch-kit
aspect: readme
schema_ref: schema/core/readme/v1
path_hint: assets/autumn-launch/readme.md
---
# 第一次接入：素材、说明与版本

本导读随 `autumn-launch-kit` r4 交付。需要已认证身份与 Dataset 消费权。
原始材料是一段视频、封面、字幕、使用范围与画面说明。先理解交付，再发布自己的内容。

```sh
kc dataset show autumn-launch-kit --revision 4
kc read 'urn:kc:object:example/autumn-launch-kit#/readme' --dataset autumn-launch-kit --revision 4
kc read 'urn:kc:object:media-asset/autumn-launch.cut-15s' --dataset autumn-launch-kit --revision 4
kc files read --dataset autumn-launch-kit --revision 4 --path _schemas/media-asset.definition.aspect.yaml
kc files read --dataset autumn-launch-kit --revision 4 --path _schemas/media-asset.vision.aspect.yaml
```

## 从需求到选择

definition、usage、vision 描述同一素材，因维护责任和更新节奏不同而成为三个方面。
视频字节是普通文件，对象负责描述它并给出下载路径。文件名和目录不决定对象身份。
若要管理多次识别任务及各自模型、输入、结果，可另建分析对象并关联素材；
当前单份 vision 适合交付一份采用的画面说明。

读取 definition 的 file、cover、subtitles 后，沿同一 Dataset 修订下载对应文件。
只有需要读取当时的外部状态才考虑 LiveAccess，下载已经发布的视频不需要它。

```sh
kc files read --dataset autumn-launch-kit --revision 4 --path assets/autumn-launch/subtitles.zh.vtt --out ./subtitles.zh.vtt
kc files read --dataset autumn-launch-kit --revision 1 --path assets/autumn-launch/subtitles.zh.vtt
kc files read --dataset autumn-launch-kit --revision 2 --path assets/autumn-launch/subtitles.zh.vtt
```

下载目标须不存在。r1 字幕为 28 元，r2 修正为 32 元；旧版保持原文。
r3 增加 vision，r4 增加本导读。新方面不要求更换素材身份，也不回写旧修订。

## 发布自己的第一份对象

```sh
kc read 'urn:kc:object:kc/guide/first-publication#/guide' --repo kr://kc/system
```

沿指南取得对象原文件和 Schema，在自己的仓发布。先判断已有结构是否能描述自己的材料；
结构相同可复用 Schema，自己的对象使用自己的稳定身份和真实正文，不能把例子价格当成业务事实。
领域对象和 Schema 经 Writer；原样视频按获准文件流程交付，不放进对象值冒充素材字节。

完成后记录接受 commit，按该 commit 读回对象和采用的 Schema，再发布自己的 Dataset。
验证消费者能读取选中的对象与文件，不能顺带读取来源仓的未选内容。

## 自测与失败处置

给同事一份“字幕更正、素材不变”的任务：他应能选择正确单元，保留素材身份，
发布新修订并解释旧版为何仍显示旧字幕。再问“每个字幕段要单独维护时怎么办”，
回答需要分析段的稳定身份与维护粒度，不应机械把所有数组改成 member。
无权时申请实际目标权限；版本不存在时核对交付；Schema 不匹配时修正资料或设计，
不能复制并改名一份同构 Schema 来绕过校验。
