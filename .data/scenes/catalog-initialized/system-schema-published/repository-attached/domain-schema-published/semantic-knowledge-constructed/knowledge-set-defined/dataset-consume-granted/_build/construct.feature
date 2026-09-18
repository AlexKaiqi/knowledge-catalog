# dataset-consume-granted：consumer 持有 scene-set 的 file.read。
# 用 --dataset 读清单内文件。不放行仓 knowledge.*。
# 主体不用 bot：其它分叉会给 bot 叠加权，隔离探会假绿。

Feature: dataset-consume-granted

  Scenario: construct
    When I run `kc grant add --principal consumer --action file.read --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | consumer |
      | catalog   | kr://scene/catalog |
      | dataset | scene-set |
      | actions.0 | file.read |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | consumer |
      | rules[].dataset | scene-set |
      | rules[].actions.0 | file.read |
    When I run `kc read --as consumer --dataset scene-set --object metric/gmv`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].commit | nonempty |
