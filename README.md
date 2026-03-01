# 🔥 Apple Ads Toolkit

Tools for structured access to Apple Ads, export/import config, apply changes, analyze data.
Use this toolkit to setup your AI-driven Apple Ads GitOps. 

```bash
go install github.com/ndx-technologies/go-apple-ads/cmd/apple-ads@latest
```

## Setup

- `<my-project>/apple-ads/config.json` — all your config is here, campaigns, adgroups, creatives
- `<my-project>/apple-ads/keywords/*.csv`
- setup apple ads custom reports. run `apple-ads merge_csv` to merge them
- after updates to keywords files run `apple-ads get update-commands-csv` to generate commands CSV file and upload it in Apple
