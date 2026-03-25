#!/usr/bin/env python3

import argparse
import csv
import os
import sys
from collections import defaultdict
from pathlib import Path

import snowballstemmer
from langdetect import DetectorFactory, LangDetectException, detect

DetectorFactory.seed = 0  # reproducible detection

COLOR = os.environ.get("NO_COLOR", "") == ""


def _set_color(val: bool) -> None:
    global COLOR
    COLOR = val


def green(s: str) -> str:
    return f"\033[32m{s}\033[0m" if COLOR else s


def red(s: str) -> str:
    return f"\033[31m{s}\033[0m" if COLOR else s


LANG_MAP = {
    "ar": "arabic",
    "da": "danish",
    "nl": "dutch",
    "en": "english",
    "fi": "finnish",
    "fr": "french",
    "de": "german",
    "hu": "hungarian",
    "it": "italian",
    "no": "norwegian",
    "pt": "portuguese",
    "ro": "romanian",
    "ru": "russian",
    "es": "spanish",
    "sv": "swedish",
    "tr": "turkish",
}

_stemmers: dict = {}


def get_stemmer(lang_code: str):
    lang = LANG_MAP.get(lang_code)
    if lang is None:
        return None
    if lang not in _stemmers:
        _stemmers[lang] = snowballstemmer.stemmer(lang)
    return _stemmers[lang]


def fingerprint(keyword: str, lang_code: str) -> tuple:
    words = keyword.lower().split()
    stemmer = get_stemmer(lang_code)
    stems = [stemmer.stemWord(w) for w in words] if stemmer else words
    return tuple(sorted(stems))


def print_table(duplicates: dict, show_id: bool) -> None:
    id_w, kw_w, bid_w, lang_w = 15, 35, 6, 4
    header = f"  {'Keyword':<{kw_w}}  {'Bid':>{bid_w}}  {'Lang':<{lang_w}}"
    divider = f"  {'-'*kw_w}  {'-'*bid_w}  {'-'*lang_w}"
    if show_id:
        header = f"  {'ID':<{id_w}}  " + header.lstrip()
        divider = f"  {'-'*id_w}  " + divider.lstrip()

    for (country, fp), rows in sorted(duplicates.items(), key=lambda x: (x[0][0], x[0][1])):
        print(f"\n{country}: [{' '.join(fp)}]")
        print(header)
        print(divider)
        for r in rows:
            kw = r["keyword"]
            if len(kw) > kw_w:
                kw = kw[: kw_w - 1] + "\u2026"
            row_str = f"  {kw:<{kw_w}}  {r['bid']:>{bid_w}}  {r['lang']:<{lang_w}}"
            if show_id:
                row_str = f"  {r['keyword_id']:<{id_w}}  " + row_str.lstrip()
            print(row_str)
    print()


def main():
    parser = argparse.ArgumentParser(
        description=(
            "Exact keywords without Search Match allow certain degree of variation."
            "This includes pluralisation and word order."
            "It is recommended to reduce such duplicates to increase apple learning engine efficiency and reduce cannibalisation."
            "https://ads.apple.com/app-store/help/keywords/0059-understand-keyword-match-types"
        )
    )
    parser.add_argument("--apple-path", default="apple-ads", help="path to dir with keywords/ subdir")
    parser.add_argument("-v", action="store_true", help="print full table")
    parser.add_argument("-id", action="store_true", help="show keyword IDs in table")
    parser.add_argument("--no-color", action="store_true", default=os.environ.get("NO_COLOR", "") != "", help="disable color output")
    args = parser.parse_args()
    _set_color(not args.no_color)

    keywords_dir = Path(args.apple_path) / "keywords"
    if not keywords_dir.is_dir():
        sys.stderr.write(red("error") + f" keywords dir not found: {keywords_dir}\n")
        sys.exit(2)

    records = []
    for filepath in sorted(keywords_dir.glob("*.csv")):
        stem = filepath.stem
        if any(s in stem for s in ("_negative")):
            continue
        country = stem

        with open(filepath, newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for row in reader:
                if row.get("Match Type") != "EXACT":
                    continue
                if row.get("Status") != "ACTIVE":
                    continue
                keyword = row["Keyword"].strip()
                if not keyword:
                    continue

                try:
                    lang = detect(keyword)
                except LangDetectException:
                    sys.stderr.write(red("error") + " cannot detect language for keyword: " + keyword + "\n")
                    continue

                fp = fingerprint(keyword, lang)
                records.append(
                    {
                        "country": country,
                        "keyword_id": row["Keyword ID"],
                        "keyword": keyword,
                        "bid": row.get("Bid", ""),
                        "lang": lang,
                        "fp": fp,
                    }
                )

    groups: dict = defaultdict(list)
    for r in records:
        groups[(r["country"], r["fp"])].append(r)

    duplicates = {k: v for k, v in groups.items() if len(v) > 1}

    if not duplicates:
        sys.stderr.write(green("ok") + f" no duplicate exact keywords ({len(records)} active exact)\n")
        return

    if args.v:
        print_table(duplicates, args.id)

    num_groups = len(duplicates)
    num_kw = sum(len(v) for v in duplicates.values())
    suffix = "" if args.v else " (run with -v for details)"
    sys.stderr.write(red("error") + f" {num_groups} duplicate group(s), {num_kw} keywords affected{suffix}\n")
    sys.exit(1)


if __name__ == "__main__":
    main()
