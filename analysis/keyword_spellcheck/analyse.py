#!/usr/bin/env python3

import argparse
import csv
import json
import os
import sys
import unicodedata
from pathlib import Path

from langdetect import DetectorFactory, LangDetectException, detect
from spellchecker import SpellChecker

DetectorFactory.seed = 0  # reproducible detection

COLOR = os.environ.get("NO_COLOR", "") == ""


def _set_color(val: bool) -> None:
    global COLOR
    COLOR = val


def green(s: str) -> str:
    return f"\033[32m{s}\033[0m" if COLOR else s


def red(s: str) -> str:
    return f"\033[31m{s}\033[0m" if COLOR else s


def yellow(s: str) -> str:
    return f"\033[33m{s}\033[0m" if COLOR else s


# pyspellchecker language codes supported
SPELL_LANGS = {"en", "es", "fr", "pt", "de", "it", "ru", "ar", "nl", "eu", "lv"}

# langdetect → pyspellchecker language code
LANG_MAP = {
    "en": "en",
    "de": "de",
    "fr": "fr",
    "it": "it",
    "pt": "pt",
    "es": "es",
    "nl": "nl",
    "ru": "ru",
    "ar": "ar",
}

# CSV filename stem prefix → pyspellchecker language code.
# None means the market uses non-Latin script — skip entirely.
COUNTRY_LANG: dict[str, str | None] = {
    "APAC": "en",
    "AR": "es",
    "AT": "de",
    "AU": "en",
    "BR": "pt",
    "CA": "en",
    "DE": "de",
    "FR": "fr",
    "HK": "en",
    "IT": "it",
    "JP": None,
    "KR": None,
    "MX": "es",
    "SG": "en",
    "UK": "en",
    "US": "en",
}

# Non-Latin script language codes — pyspellchecker cannot check these
NON_LATIN = {"ja", "ko", "zh-cn", "zh-tw", "zh", "hi", "th", "vi", "ka", "am", "my"}

_checkers: dict = {}


def get_checker(lang_code: str):
    if lang_code not in _checkers:
        _checkers[lang_code] = SpellChecker(language=lang_code)
    return _checkers[lang_code]


def has_non_latin(text: str) -> bool:
    """Return True if the text contains characters outside the Latin script block."""
    for ch in text:
        if ch.isspace() or not ch.isalpha():
            continue
        try:
            name = unicodedata.name(ch, "")
        except (ValueError, TypeError):
            return True
        if not any(name.startswith(p) for p in ("LATIN", "BASIC LATIN")):
            return True
    return False


def load_paused_ids(apple_path: Path) -> tuple[set, set]:
    """Return (paused_campaign_ids, paused_adgroup_ids) from config.json."""
    config_path = apple_path / "config.json"
    if not config_path.exists():
        return set(), set()
    with open(config_path) as f:
        config = json.load(f)
    paused_campaigns: set = set()
    paused_adgroups: set = set()
    for campaign in config.get("campaigns", []):
        cid = campaign.get("id")
        if campaign.get("status") != "ENABLED":
            if cid:
                paused_campaigns.add(cid)
        for ag in campaign.get("adgroups", []):
            agid = ag.get("id")
            if not agid:
                continue
            if ag.get("status") != "ENABLED" or cid in paused_campaigns:
                paused_adgroups.add(agid)
    return paused_campaigns, paused_adgroups


def check_keyword(keyword: str, lang_code: str) -> list[tuple[str, list[str]]]:
    """
    Return list of (misspelled_word, [suggestions]) for each bad word.
    Returns empty list if all words are spelled correctly.
    """
    spell = get_checker(lang_code)
    words = keyword.lower().split()
    results = []
    unknown = spell.unknown(words)
    for word in words:
        if word in unknown:
            candidates = list(spell.candidates(word) or [])
            # exclude the word itself from suggestions
            suggestions = [c for c in candidates if c != word][:3]
            results.append((word, suggestions))
    return results


def country_from_stem(stem: str) -> str:
    """Extract the base country code from a filename stem like 'US_discovery' → 'US'."""
    return stem.split("_")[0]


def main():
    parser = argparse.ArgumentParser(description="Check active Apple Ads keywords for spelling errors.")
    parser.add_argument("--apple-path", default="apple-ads", help="path to dir with keywords/ subdir and config.json")
    parser.add_argument("-v", action="store_true", help="verbose: show suggestions")
    parser.add_argument("-id", action="store_true", help="show keyword IDs")
    parser.add_argument(
        "--detect-language",
        action="store_true",
        help="use langdetect to infer language from keyword text instead of filename-based mapping",
    )
    parser.add_argument(
        "--no-color",
        action="store_true",
        default=os.environ.get("NO_COLOR", "") != "",
        help="disable color output",
    )
    args = parser.parse_args()
    _set_color(not args.no_color)

    apple_path = Path(args.apple_path)
    keywords_dir = apple_path / "keywords"
    if not keywords_dir.is_dir():
        sys.stderr.write(red("error") + f" keywords dir not found: {keywords_dir}\n")
        sys.exit(2)

    _, paused_adgroups = load_paused_ids(apple_path)

    misspellings: list[dict] = []
    total_checked = 0
    total_skipped_lang = 0

    for filepath in sorted(keywords_dir.glob("*.csv")):
        stem = filepath.stem
        # skip negatives and competitor lists (brand names are intentional)
        if any(s in stem for s in ("_negative", "_competitors")):
            continue
        country = stem

        if args.detect_language:
            spell_lang_for_file = None  # determined per-keyword below
        else:
            base_country = country_from_stem(stem)
            if base_country not in COUNTRY_LANG:
                # unknown market: skip
                continue
            spell_lang_for_file = COUNTRY_LANG[base_country]
            if spell_lang_for_file is None:
                # non-Latin market
                continue

        with open(filepath, newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for row in reader:
                if row.get("Status") != "ACTIVE":
                    continue
                keyword = row.get("Keyword", "").strip()
                if not keyword:
                    continue
                if row.get("Ad Group ID", "") in paused_adgroups:
                    continue

                if args.detect_language:
                    # skip non-Latin scripts early
                    if has_non_latin(keyword):
                        total_skipped_lang += 1
                        continue
                    try:
                        lang = detect(keyword)
                    except LangDetectException:
                        total_skipped_lang += 1
                        continue
                    spell_lang = LANG_MAP.get(lang)
                    if spell_lang is None:
                        total_skipped_lang += 1
                        continue
                else:
                    # skip non-Latin keywords even in Latin-script markets (e.g. mixed HK)
                    if has_non_latin(keyword):
                        total_skipped_lang += 1
                        continue
                    lang = spell_lang_for_file
                    spell_lang = spell_lang_for_file

                total_checked += 1
                errors = check_keyword(keyword, spell_lang)
                if errors:
                    misspellings.append(
                        {
                            "country": country,
                            "keyword_id": row.get("Keyword ID", ""),
                            "keyword": keyword,
                            "bid": row.get("Bid", ""),
                            "lang": lang,
                            "errors": errors,
                        }
                    )

    if not misspellings:
        sys.stderr.write(green("ok") + f" no misspellings ({total_checked} keywords checked, {total_skipped_lang} skipped non-latin/unsupported)\n")
        return

    # Print results
    kw_w, id_w, bid_w, lang_w = 40, 15, 6, 4
    header = f"  {'Keyword':<{kw_w}}  {'Bid':>{bid_w}}  {'Lang':<{lang_w}}  Misspelled"
    if args.id:
        header = f"  {'ID':<{id_w}}  " + header.lstrip()

    prev_country = None
    for entry in sorted(misspellings, key=lambda x: (x["country"], x["keyword"])):
        if entry["country"] != prev_country:
            print(f"\n{entry['country']}:")
            print(header)
            print(f"  {'-'*kw_w}  {'-'*bid_w}  {'-'*lang_w}  {'-'*30}")
            prev_country = entry["country"]

        kw = entry["keyword"]
        if len(kw) > kw_w:
            kw = kw[: kw_w - 1] + "\u2026"

        error_parts = []
        for word, suggestions in entry["errors"]:
            if args.v and suggestions:
                error_parts.append(f"{yellow(word)} → {', '.join(suggestions)}")
            else:
                error_parts.append(yellow(word))
        error_str = "  ".join(error_parts)

        row_str = f"  {kw:<{kw_w}}  {entry['bid']:>{bid_w}}  {entry['lang']:<{lang_w}}  {error_str}"
        if args.id:
            row_str = f"  {entry['keyword_id']:<{id_w}}  " + row_str.lstrip()
        print(row_str)

    print()
    sys.stderr.write(red("error") + f" {len(misspellings)} keyword(s) with misspellings" f" ({total_checked} checked, {total_skipped_lang} skipped)\n")
    sys.exit(1)


if __name__ == "__main__":
    main()
