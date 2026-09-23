# cld_generic_dangeon

## ビルドとテスト

```
go build ./... && go vet ./... && go test -race ./...
```

ベンチマーク:

```
go test ./engine -run '^$' -bench WorldStep
```

## 実験

```
go run ./cmd/experiment -map flat -w 64 -h 48 -seeds 12 -ticks 10000
```

| オプション | 意味 |
| --- | --- |
| `-map` | 地図。`flat`（全面が陸・地域1つ）/ `constrained`（地域4つ・陸を二分する水の帯） |
| `-w` / `-h` | 地図の幅・高さ（タイル数）。必須 |
| `-seeds` | シード数。必須 |
| `-seed0` | 最初のシード（既定 1） |
| `-ticks` | 1回の実行の tick 数。必須 |
| `-variants` | 比べる条件をカンマ区切りで。先頭が base（既定 `base`） |

標準出力に `MEASURE.md` の形式のレポート（§2〜§9 と補助の表）を出す。§1 概要は人が書く。シードごとの実行時間は標準エラーに出す。保存則の検算が合わない tick があれば、その場で止まる。

## 実装前の数え上げ

```
go run ./cmd/count -what reach -map flat -w 64 -h 48 -seeds 12
```

| `-what` | 数えるもの |
| --- | --- |
| `reach` | 食料だけの世界での、最寄りの食料までの距離と到達比 |
