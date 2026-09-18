# Authentication

定位：整理稿。能力简介，正文后续梳理。

请求上出现经过验证的 principal。它只回答「你是谁」，不回答「你能做什么」。

**独立验收。** authenticator 配对：local / Taihu / Gitea；拒绝自报身份。

**不依赖也能说清的失败。** 未声明模式静默变成 local；`X-Kc-As` 被 taihu 接受。

**明确不是。** 授权表、允许动作。
