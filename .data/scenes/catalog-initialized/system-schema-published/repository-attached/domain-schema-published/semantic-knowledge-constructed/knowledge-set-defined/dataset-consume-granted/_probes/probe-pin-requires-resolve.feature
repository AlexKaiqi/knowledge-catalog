# 在 dataset-consume-granted 上：产品 argv 不接受 --pin。file.read 走 --dataset。

Feature: probe pin is not a product flag

  Scenario: consume rejects --pin
    When I run `kc read --as consumer --pin $home/consume-pin.json --object metric/gmv`
    Then error USAGE_INVALID
