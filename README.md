# 🔥 Apple Ads Toolkit

Tools for structured access to Apple Ads, export/import config, apply changes, analyze data.
Use this toolkit to setup your AI-driven Apple Ads GitOps. 

## Setup

- `apple-ads/config.json` — all your config is here, campaigns, adgroups, creatives
- `apple-ads/keywords/*.csv`
- setup apple ads custom reports. run `apple_ads_merge_csv` to merge them
- after updates, run `apple_ads_get_update_commands_csv` to generate commands CSV file and upload it to Apple
