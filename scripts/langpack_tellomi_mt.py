#!/usr/bin/env python3
"""Translate Tellomi strings that are still a copy of the English text.

Short labels reuse the wording already in that pack (Settings, Contacts,
Followers, and the other official keys listed below). Time-ago lines reuse
the pack's plural form, with the Tellomi placeholder left as it is. Everything
else is machine-translated. Placeholders are not sent to the translator.

The script is safe to run twice. Android packs move from 61730401 to
61730402 once. Each iOS pack moves forward by one from the versions shipped
in #297. android_xml_fix_version stays 61730400. lang_code and plural_code
are not written.

Strings that stay identical to English are recorded in
data/langpack/tellomi-english-allowlist.json. A later gap check fails when a
Tellomi value equals English and is not on that list.
"""

from __future__ import annotations

import gzip
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from langpack_fill import (  # noqa: E402
    GOOGLE,
    Translator,
    placeholders,
    same_tokens,
    stitch,
)
from langpack_gap import load_doc, manifest_keys, write_gz  # noqa: E402
from langpack_official import android_entry_safe  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent / "data" / "langpack"
ALLOWLIST_NAME = "tellomi-english-allowlist.json"
ANDROID_TARGET = 61730402
ANDROID_PREVIOUS = 61730401
ANDROID_FIX = 61730400

# iOS pack versions after #297. A rerun does not add another one.
IOS_BASE = {
    "ar": 22452757,
    "be": 22420169,
    "ca": 22412797,
    "cs": 22418778,
    "de": 22431601,
    "el": 22202258,
    "en": 22347349,
    "es": 22412199,
    "fa": 22437412,
    "fi": 22410853,
    "fr": 22391558,
    "he": 22452941,
    "hi": 22202258,
    "id": 22321174,
    "it": 22388317,
    "ja": 22418204,
    "ko": 22454169,
    "ms": 22352690,
    "nb": 22420482,
    "nl": 22449821,
    "pl": 22392157,
    "pt-br": 22431432,
    "ro": 22444906,
    "ru": 22421466,
    "sv": 22427913,
    "th": 22202258,
    "tr": 22434108,
    "uk": 22460355,
    "uz": 22449132,
    "vi": 22347889,
    "zh-hans": 22454351,
    "zh-hant": 22421536,
}

# Whole values that are names, not translations.
BRANDS = ("PDF", "Tellomi", "TELLOMI")

# Single title-case words a translator may return unchanged. A sentence is
# never kept just because it came back in English. Each language that keeps
# the word is recorded on the allowlist.
LOANWORDS = {"OK", "Spam", "System", "Video", "Menu", "PDF", "Top"}

# Feature name for the Moments tab. French and Catalan use the same word as
# English; those keys are allowlisted. Chinese is already translated.
PRODUCT = {
    "ar": "اللحظات",
    "be": "Моманты",
    "ca": "Moments",
    "cs": "Momenty",
    "de": "Momente",
    "el": "Στιγμές",
    "es": "Momentos",
    "fa": "لحظه‌ها",
    "fi": "Hetket",
    "fr": "Moments",
    "he": "רגעים",
    "hi": "मोमेंट्स",
    "id": "Momen",
    "it": "Momenti",
    "ja": "モーメント",
    "ko": "모멘트",
    "ms": "Momen",
    "nb": "Øyeblikk",
    "nl": "Momenten",
    "pl": "Momenty",
    "pt-br": "Momentos",
    "ro": "Momente",
    "ru": "Моменты",
    "sv": "Ögonblick",
    "th": "โมเมนต์",
    "tr": "Anlar",
    "uk": "Моменти",
    "uz": "Lahzalar",
    "vi": "Khoảnh khắc",
    "zh-hans": "动态",
    "zh-hant": "動態",
}

# City names. A value equal to English is a proper noun this language writes
# the same way, and the allowlist records it.
CITIES = {
    "Moments_City_Beijing": {
        "ar": "بكين",
        "be": "Пекін",
        "ca": "Pequín",
        "cs": "Peking",
        "de": "Peking",
        "el": "Πεκίνο",
        "es": "Pekín",
        "fa": "پکن",
        "fi": "Peking",
        "fr": "Pékin",
        "he": "בייג'ינג",
        "hi": "बीजिंग",
        "id": "Beijing",
        "it": "Pechino",
        "ja": "北京",
        "ko": "베이징",
        "ms": "Beijing",
        "nb": "Beijing",
        "nl": "Peking",
        "pl": "Pekin",
        "pt-br": "Pequim",
        "ro": "Beijing",
        "ru": "Пекин",
        "sv": "Peking",
        "th": "ปักกิ่ง",
        "tr": "Pekin",
        "uk": "Пекін",
        "uz": "Pekin",
        "vi": "Bắc Kinh",
    },
    "Moments_City_Shanghai": {
        "ar": "شنغهاي",
        "be": "Шанхай",
        "ca": "Xangai",
        "cs": "Šanghaj",
        "de": "Shanghai",
        "el": "Σαγκάη",
        "es": "Shanghái",
        "fa": "شانگهای",
        "fi": "Shanghai",
        "fr": "Shanghai",
        "he": "שנגחאי",
        "hi": "शंघाई",
        "id": "Shanghai",
        "it": "Shanghai",
        "ja": "上海",
        "ko": "상하이",
        "ms": "Shanghai",
        "nb": "Shanghai",
        "nl": "Shanghai",
        "pl": "Szanghaj",
        "pt-br": "Xangai",
        "ro": "Shanghai",
        "ru": "Шанхай",
        "sv": "Shanghai",
        "th": "เซี่ยงไฮ้",
        "tr": "Şanghay",
        "uk": "Шанхай",
        "uz": "Shanxay",
        "vi": "Thượng Hải",
    },
    "Moments_City_Chengdu": {
        "ar": "تشنغدو",
        "be": "Чэнду",
        "ca": "Chengdu",
        "cs": "Čcheng-tu",
        "de": "Chengdu",
        "el": "Τσενγκντού",
        "es": "Chengdu",
        "fa": "چنگدو",
        "fi": "Chengdu",
        "fr": "Chengdu",
        "he": "צ'נגדו",
        "hi": "चेंगदू",
        "id": "Chengdu",
        "it": "Chengdu",
        "ja": "成都",
        "ko": "청두",
        "ms": "Chengdu",
        "nb": "Chengdu",
        "nl": "Chengdu",
        "pl": "Chengdu",
        "pt-br": "Chengdu",
        "ro": "Chengdu",
        "ru": "Чэнду",
        "sv": "Chengdu",
        "th": "เฉิงตู",
        "tr": "Chengdu",
        "uk": "Ченду",
        "uz": "Chengdu",
        "vi": "Thành Đô",
    },
    "Moments_City_Guangzhou": {
        "ar": "قوانغتشو",
        "be": "Гуанчжоу",
        "ca": "Guangzhou",
        "cs": "Kanton",
        "de": "Kanton",
        "el": "Γκουανγκτζόου",
        "es": "Cantón",
        "fa": "گوانگژو",
        "fi": "Guangzhou",
        "fr": "Canton",
        "he": "גואנגג'ואו",
        "hi": "गुआंगझोउ",
        "id": "Guangzhou",
        "it": "Canton",
        "ja": "広州",
        "ko": "광저우",
        "ms": "Guangzhou",
        "nb": "Guangzhou",
        "nl": "Guangzhou",
        "pl": "Kanton",
        "pt-br": "Guangzhou",
        "ro": "Guangzhou",
        "ru": "Гуанчжоу",
        "sv": "Guangzhou",
        "th": "กว่างโจว",
        "tr": "Guangzhou",
        "uk": "Гуанчжоу",
        "uz": "Guanchjou",
        "vi": "Quảng Châu",
    },
    "Moments_City_Hangzhou": {
        "ar": "هانغتشو",
        "be": "Ханчжоу",
        "ca": "Hangzhou",
        "cs": "Chang-čou",
        "de": "Hangzhou",
        "el": "Χανγκτζόου",
        "es": "Hangzhou",
        "fa": "هانگژو",
        "fi": "Hangzhou",
        "fr": "Hangzhou",
        "he": "האנגג'ואו",
        "hi": "हांगझोउ",
        "id": "Hangzhou",
        "it": "Hangzhou",
        "ja": "杭州",
        "ko": "항저우",
        "ms": "Hangzhou",
        "nb": "Hangzhou",
        "nl": "Hangzhou",
        "pl": "Hangzhou",
        "pt-br": "Hangzhou",
        "ro": "Hangzhou",
        "ru": "Ханчжоу",
        "sv": "Hangzhou",
        "th": "หางโจว",
        "tr": "Hangzhou",
        "uk": "Ханчжоу",
        "uz": "Xanchjou",
        "vi": "Hàng Châu",
    },
    "Moments_City_Shenzhen": {
        "ar": "شنتشن",
        "be": "Шэньчжэнь",
        "ca": "Shenzhen",
        "cs": "Šen-čen",
        "de": "Shenzhen",
        "el": "Σεντζέν",
        "es": "Shenzhen",
        "fa": "شنژن",
        "fi": "Shenzhen",
        "fr": "Shenzhen",
        "he": "שנג'ן",
        "hi": "शेनझेन",
        "id": "Shenzhen",
        "it": "Shenzhen",
        "ja": "深圳",
        "ko": "선전",
        "ms": "Shenzhen",
        "nb": "Shenzhen",
        "nl": "Shenzhen",
        "pl": "Shenzhen",
        "pt-br": "Shenzhen",
        "ro": "Shenzhen",
        "ru": "Шэньчжэнь",
        "sv": "Shenzhen",
        "th": "เซินเจิ้น",
        "tr": "Shenzhen",
        "uk": "Шеньчжень",
        "uz": "Shenchjen",
        "vi": "Thâm Quyến",
    },
}

# Official keys whose English is a short UI word. The first key whose English
# equals the Tellomi string wins. Tellomi keys are not in this list.
GLOSSARY_KEYS = {
    "android": [
        "Settings",
        "Contacts",
        "MainTabsChats",
        "MainTabsCalls",
        "MainTabsProfile",
        "Cancel",
        "Delete",
        "Edit",
        "Save",
        "Done",
        "Close",
        "Search",
        "Send",
        "Copy",
        "Forward",
        "Reply",
        "Comment",
        "OK",
        "Stop",
        "Poll",
        "Quote",
        "Story",
        "Everyone",
        "AttachPhoto",
        "AttachVideo",
        "AttachSticker",
        "AttachDocument",
        "AttachArticle",
        "ReportChatSpam",
        "ReportChatPornography",
        "ReportChatViolence",
        "ReportChatOther",
        "RightNow",
        "Loading",
        "FollowersChartTitle",
        "PremiumMore",
        "ShareFile",
        "FilterAllChatsShort",
        "BotsMenuTitle",
        "CommentsTitle",
        "ArticleHintAuthor",
        "GroupCallCreateVoice",
        "SuggestedPostPublish",
        "InteractionsChartTitle",
        "NotificationsCustom",
    ],
    "ios": [
        "Settings.Title",
        "Contacts.Title",
        "DialogList.Title",
        "Calls.TabTitle",
        "Common.Cancel",
        "Common.Delete",
        "Conversation.Edit",
        "Common.Save",
        "Common.Done",
        "Common.Close",
        "Common.Search",
        "Notification.Reply",
        "Stats.Followers",
        "Share.Title",
        "Common.Back",
        "Chat.PostSuggestion.Approve.Action",
        "VoiceOver.Chat.Stop",
    ],
}

# Fixes for strings the translator echoed in English, or for a short label
# whose pack has no official key. Checked before glossary and Google.
MANUAL = {
    ("android", "ca", "Moments_Repost"): "Republica",
    ("android", "ca", "Moments_FilterRepostsOnly"): "Republicacions",
    ("android", "ca", "Moments_Filter_Reposts"): "Republicacions",
    ("ios", "ca", "Moments.Repost"): "Republica",
    ("ios", "ca", "Moments.Profile.Reposts"): "Republicacions",
    ("android", "cs", "Moments_Repost"): "Sdílet znovu",
    ("android", "cs", "Moments_FilterRepostsOnly"): "Sdílené",
    ("android", "cs", "Moments_Filter_Reposts"): "Sdílené",
    ("android", "cs", "Moments_Sort_Relevance"): "Relevantnost",
    ("ios", "cs", "Moments.Repost"): "Sdílet znovu",
    ("ios", "cs", "Moments.Profile.Reposts"): "Sdílené",
    ("ios", "cs", "Moments.Sort.Relevance"): "Relevantnost",
    ("android", "de", "Moments_Repost"): "Erneut teilen",
    ("android", "de", "Moments_FilterRepostsOnly"): "Erneut geteilt",
    ("android", "de", "Moments_Filter_Reposts"): "Erneut geteilt",
    ("android", "de", "Moments_Draft_Message"): "Du hast das noch nicht gepostet.",
    ("android", "de", "Moments_EmptyFriends"): "Folgt einander, dann seid ihr befreundet. Ihre Beiträge erscheinen hier.",
    ("android", "de", "Moments_Empty_Following"): "Leute, denen du folgst, haben noch nichts gepostet.",
    ("android", "de", "Moments_Empty_Nearby"): "In dieser Stadt gibt es noch keine Beiträge.",
    ("ios", "de", "Moments.Repost"): "Erneut teilen",
    ("ios", "de", "Moments.Profile.Reposts"): "Erneut geteilt",
    ("ios", "de", "Moments.Empty.Following"): "Leute, denen du folgst, haben noch nichts gepostet.",
    ("ios", "de", "Moments.Empty.Friends"): "Folgt einander, dann seid ihr befreundet. Ihre Beiträge erscheinen hier.",
    ("ios", "de", "Moments.Empty.Nearby"): "In dieser Stadt gibt es noch keine Beiträge.",
    ("ios", "fr", "Moments.Notif.Interactions"): "Interactions",
    ("android", "fi", "Moments_Report_Fake"): "Väärennös",
    ("android", "sv", "Moments_Report_Fake"): "Förfalskning",
    ("android", "id", "Moments_Uploading"): "Memposting",
    ("ios", "id", "Moments.Uploading"): "Memposting",
    ("android", "nb", "Moments_Repost"): "Del på nytt",
    ("android", "nb", "Moments_SendOriginal"): "Send originalen",
    ("ios", "nb", "Moments.Repost"): "Del på nytt",
    ("android", "uz", "Moments_Repost"): "Qayta joylash",
    ("ios", "uz", "Moments.Repost"): "Qayta joylash",
}


# These keys share an English word with another control but mean something else.
# The value is the English we translate instead of the key's own text.
GLOSS_OF_KEY = {
    ("android", "Moments_FollowingState"): "Already following",
    ("ios", "Moments.FollowingState"): "Already following",
    ("android", "Moments_Publish"): "Publish",
    ("ios", "Moments.Publish"): "Publish",
}

# Single format strings. The client does not ask for one/few/many on these
# keys, so the translation is one form: "many" when the pack pluralizes that
# way, otherwise "other". The placeholder is rewritten to the Tellomi one.
TIME_KEY = {
    "Moments_Time_Days": "DaysAgo",
    "Moments_Time_Hours": "HoursAgo",
    "Moments_Time_Minutes": "MinutesAgo",
    "Moments_Time_Months": "MonthsAgo",
    "Moments_Time_Years": "YearsAgo",
}

INDIC_DIGITS = str.maketrans("٠١٢٣٤٥٦٧٨٩۰۱۲۳۴۵۶۷۸۹０１２３４５６７８９", "012345678901234567890123456789")
INT_PLACEHOLDER = re.compile(r"%(\d+\$)?[diu]")


def text_of(item: dict | None) -> str:
    if not item:
        return ""
    return item.get("value") or ""


def shouty(text: str) -> bool:
    letters = [ch for ch in text if ch.isalpha()]
    return len(letters) >= 3 and all(ch.isupper() for ch in letters)


def digit_runs(text: str) -> list[str]:
    shielded = re.sub(
        r"%(?:\d+\$)?[#0\- +']*(?:\d+|\*)?(?:\.\d+)?(?:hh|h|ll|l|z|j|t|q|L)?[diuoxXfFeEgGaAcspn@]",
        " ",
        text or "",
    )
    return re.findall(r"\d+", shielded)


def tidy(text: str) -> str:
    text = (text or "").translate(INDIC_DIGITS)
    text = text.replace("％", "%")
    text = re.sub(r"%\s+(\d+\$)?\s*", lambda m: "%" + (m.group(1) or ""), text)
    text = text.replace("&amp;", "&")
    # A bare & is not XML-safe. "& " is what Android escapeString accepts.
    text = re.sub(r"&(?![\s#a-zA-Z])", "＆", text)
    text = re.sub(r"&(?=[A-Za-z]+;)", lambda m: m.group(0), text)
    return text.strip()


def adapt_placeholder(text: str, english: str) -> str | None:
    """Rewrite the official plural's number token to the Tellomi placeholder."""
    if not text or placeholders(text) == placeholders(english) and same_tokens(english, text):
        return text if text else None
    src = INT_PLACEHOLDER.findall(text)
    # findall above returns only the optional index. Count with finditer.
    src_tokens = [m.group(0) for m in INT_PLACEHOLDER.finditer(text)]
    dst_tokens = [m.group(0) for m in INT_PLACEHOLDER.finditer(english)]
    if len(src_tokens) != 1 or len(dst_tokens) != 1:
        return None
    rewritten = text.replace(src_tokens[0], dst_tokens[0], 1)
    if not same_tokens(english, rewritten):
        return None
    return rewritten


def pick_time_form(item: dict) -> str | None:
    """General plural that still contains a number.

    Languages that distinguish "many" use it, because the English source is
    the plural ("days", not "day"). Other languages use "other".
    """
    one = item.get("one") or ""
    many = item.get("many") or ""
    other = item.get("other") or ""

    def has_number(value: str) -> bool:
        return INT_PLACEHOLDER.search(value or "") is not None

    if many and many != one and has_number(many):
        return many
    if has_number(other):
        return other
    for form in ("few", "two", "one", "zero"):
        value = item.get(form) or ""
        if has_number(value):
            return value
    return None


def clean_label(text: str) -> str:
    text = (text or "").strip().strip('"').strip()
    if not text or '"' in text:
        return ""
    return text


def build_glossary(en_items: dict[str, dict], target_items: dict[str, dict], platform: str) -> dict[str, tuple[str, str]]:
    """English label -> (this pack's translation, official key)."""
    out: dict[str, tuple[str, str]] = {}
    for key in GLOSSARY_KEYS[platform]:
        english = clean_label(text_of(en_items.get(key)))
        translated = clean_label(text_of(target_items.get(key)))
        if not english or not translated:
            continue
        en_item = en_items.get(key) or {}
        if (en_item.get("one") or en_item.get("other")) and not en_item.get("value"):
            continue
        if shouty(translated):
            continue
        # A one-word English label should not pick up a sentence.
        if " " not in english and len(translated) > 40:
            continue
        if english not in out:
            out[english] = (translated, key)
    return out


_AT_TOKEN = "TELLOMI"
_AT_RE = re.compile(r"@(?:%\d+\$[@sdif]|%[@sdif])")


def protect_product(text: str) -> tuple[str, int]:
    count = len(re.findall(r"\bMoments\b", text))
    return re.sub(r"\bMoments\b", "Tellomi", text), count


def shield_ats(text: str) -> tuple[str, list[str]]:
    """Keep @%s and @%@ together so a translator cannot move the @."""
    slots: list[str] = []

    def repl(match: re.Match[str]) -> str:
        slots.append(match.group(0))
        return _AT_TOKEN

    return _AT_RE.sub(repl, text), slots


def restore_ats(text: str, slots: list[str]) -> str | None:
    if text.count(_AT_TOKEN) != len(slots):
        return None
    for slot in slots:
        text = text.replace(_AT_TOKEN, slot, 1)
    return text


def restore_product(text: str, name: str, count: int) -> str | None:
    if text.count("Tellomi") != count:
        return None
    return text.replace("Tellomi", name)


class Engine:
    def __init__(self, translator: Translator) -> None:
        self.translator = translator
        self.problems: list[str] = []

    def google(self, code: str, english: str) -> str | None:
        tl = GOOGLE.get(code)
        if not tl:
            return None
        shielded, moments = protect_product(english)
        shielded, ats = shield_ats(shielded)
        pieces = _pieces(shielded)
        missing = [(tl, piece) for piece in pieces if self.translator.get(tl, piece) is None]
        if missing:
            self.translator.translate_all(missing)
        got = tidy(stitch(shielded, tl, self.translator))
        if ats:
            restored = restore_ats(got, ats)
            if restored is None:
                return None
            got = restored
        if moments:
            restored = restore_product(got, PRODUCT[code], moments)
            if restored is None:
                return None
            got = restored
        return got


def _pieces(text: str) -> list[str]:
    from langpack_fill import pieces_of

    return pieces_of(text)


def acceptable(platform: str, english: str, translated: str) -> bool:
    if not translated or not translated.strip():
        return False
    if not same_tokens(english, translated):
        return False
    if digit_runs(english) != digit_runs(translated):
        return False
    for token in ("@%s", "@%@", "%1$s", "%1$d", "%1$@"):
        if english.count(token) != translated.count(token):
            return False
    if platform == "android" and not android_entry_safe({"key": "K", "value": translated}):
        return False
    if any(ord(ch) < 32 and ch not in "\t\n\r" for ch in translated):
        return False
    return True


def choose(
    engine: Engine,
    platform: str,
    code: str,
    key: str,
    english: str,
    glossary: dict[str, tuple[str, str]],
    siblings: dict[str, str],
    official: dict[str, dict],
) -> tuple[str, str, str | None] | None:
    """Return (value, via, allow_reason). allow_reason is set when the value stays English."""
    manual = MANUAL.get((platform, code, key))
    if manual is not None:
        if not acceptable(platform, english, manual):
            return None
        reason = "same word in this language" if manual == english else None
        return manual, "manual", reason

    if english in BRANDS:
        return english, "brand", "brand name"

    if key in CITIES and code in CITIES[key]:
        value = CITIES[key][code]
        reason = "proper noun written the same way in this language" if value == english else None
        return value, "city", reason

    if english == "Moments" or key == "Moments_DetailTitle":
        value = PRODUCT[code]
        # DetailTitle's English is "Post". The shipped Chinese title is the
        # feature name, so every language uses that same feature name.
        same = value == english
        reason = "product name is the same word in this language" if same else None
        return value, "product", reason

    gloss = GLOSS_OF_KEY.get((platform, key))
    if gloss == "Publish":
        publish_key = "SuggestedPostPublish" if platform == "android" else "Chat.PostSuggestion.Approve.Action"
        item = official.get(publish_key) or {}
        value = text_of(item)
        if value and value != "Publish" and not shouty(value) and acceptable(platform, english, value):
            return value, "glossary:" + publish_key, None
        gloss = "Publish"

    if gloss:
        got = engine.google(code, gloss)
        if got and got != english and acceptable(platform, english, got):
            return got, "google", None
        return None

    if english in siblings:
        value = siblings[english]
        if acceptable(platform, english, value):
            reason = "same wording as another Tellomi string in this pack" if value == english else None
            return value, "sibling", reason

    if key in TIME_KEY:
        form = pick_time_form(official.get(TIME_KEY[key]) or {})
        adapted = adapt_placeholder(form or "", english) if form else None
        # An official plural that is still the English sentence is not a translation.
        if adapted and adapted != english and acceptable(platform, english, adapted):
            return adapted, "plural:" + TIME_KEY[key], None

    # "Followers · %d" keeps the pack's word for Followers.
    for label, splitter in (("Followers · %d", "Followers"), ("Following · %d", "Following")):
        if english == label and splitter in glossary:
            term, source = glossary[splitter]
            value = f"{term} · %d"
            if acceptable(platform, english, value):
                reason = f"same as {source}" if value == english else None
                return value, "glossary:" + source, reason

    if english in glossary and gloss is None:
        value, source = glossary[english]
        if acceptable(platform, english, value):
            reason = f"same as {source}" if value == english else None
            return value, "glossary:" + source, reason

    got = engine.google(code, english)
    if got and acceptable(platform, english, got) and got != english:
        return got, "google", None
    if got == english and english in LOANWORDS and " " not in english:
        return got, "loanword", "loanword this language writes the same way"
    return None


def pack_version(platform: str, code: str, current: int) -> int:
    if platform == "android":
        if current not in (ANDROID_PREVIOUS, ANDROID_TARGET):
            raise SystemExit(f"android {code} version {current}, want {ANDROID_PREVIOUS} or {ANDROID_TARGET}")
        return ANDROID_TARGET
    base = IOS_BASE.get(code)
    if base is None:
        raise SystemExit(f"ios {code} has no base version")
    if current == base:
        return base + 1
    if current == base + 1:
        return current
    raise SystemExit(f"ios {code} version {current}, want {base} or {base + 1}")


def apply_language(
    engine: Engine,
    platform: str,
    code: str,
    keys: list[str],
    en_items: dict[str, dict],
    official_items: dict[str, dict],
) -> tuple[dict, list[dict], dict[str, int]]:
    path = ROOT / platform / f"{code}.json.gz"
    doc = load_doc(path)
    if doc.get("lang_code") != code:
        raise SystemExit(f"{platform}/{code} lang_code {doc.get('lang_code')}")
    if platform == "android" and doc.get("android_xml_fix_version") != ANDROID_FIX:
        raise SystemExit(f"android {code} fix {doc.get('android_xml_fix_version')}")
    version = pack_version(platform, code, int(doc.get("version") or 0))
    by_key = {item["key"]: item for item in doc["strings"]}
    glossary = build_glossary(en_items, by_key, platform)
    siblings: dict[str, set[str]] = {}
    for key in keys:
        item = by_key.get(key)
        english = text_of(en_items.get(key))
        current = text_of(item)
        if not english or current == english:
            continue
        if (platform, key) in GLOSS_OF_KEY:
            continue
        siblings.setdefault(english, set()).add(current)
    sibling_one = {english: next(iter(values)) for english, values in siblings.items() if len(values) == 1}

    counts = {"changed": 0, "already": 0, "allow": 0, "google": 0, "glossary": 0, "plural": 0, "product": 0, "city": 0, "sibling": 0, "manual": 0}
    allow_rows: list[dict] = []
    for key in keys:
        item = by_key.get(key)
        if item is None:
            engine.problems.append(f"{platform}/{code} missing {key}")
            continue
        english = text_of(en_items.get(key))
        if not english:
            engine.problems.append(f"{platform}/{code} {key} has no English")
            continue
        current = text_of(item)
        if code == "en":
            if current != english:
                engine.problems.append(f"en {platform} {key} is not the English source")
            continue
        if current != english:
            counts["already"] += 1
            continue
        picked = choose(engine, platform, code, key, english, glossary, sibling_one, official_items)
        if picked is None:
            engine.problems.append(f"{platform}/{code} {key} untranslated {english!r}")
            continue
        value, via, reason = picked
        if value == english:
            if not reason and english not in BRANDS:
                engine.problems.append(f"{platform}/{code} {key} still English")
                continue
            counts["allow"] += 1
            allow_rows.append(
                {
                    "platform": platform,
                    "code": code,
                    "key": key,
                    "value": value,
                    "reason": reason or "brand name",
                }
            )
            continue
        if not acceptable(platform, english, value):
            engine.problems.append(f"{platform}/{code} {key} rejected {value!r}")
            continue
        item["value"] = value
        item["version"] = version
        item["mt"] = True
        item["mt_via"] = via.split(":", 1)[0]
        counts["changed"] += 1
        bucket = via.split(":", 1)[0]
        if bucket in counts:
            counts[bucket] += 1
        else:
            counts["google"] += 1
    doc["version"] = version
    return doc, allow_rows, counts


def update_reviewed(platform: str, code: str, changed: dict[str, tuple[str, int, str]]) -> None:
    path = ROOT / "reviewed" / platform / f"{code}.json.gz"
    if not path.exists():
        return
    doc = load_doc(path)
    for item in doc.get("strings") or []:
        key = item.get("key")
        if key not in changed:
            continue
        value, version, via = changed[key]
        item["value"] = value
        item["version"] = version
        item["pin"] = True
        item["mt"] = True
        item["mt_via"] = via
    write_gz(path, doc)


def update_machine(platform: str, code: str, rows: list[dict]) -> None:
    path = ROOT / "machine" / platform / f"{code}.json.gz"
    if path.exists():
        doc = load_doc(path)
    else:
        doc = {
            "lang_code": code,
            "platform": platform,
            "note": "Machine translations for review. mt:true. Placeholders were not sent to the translator.",
            "strings": [],
        }
    by_key = {item["key"]: item for item in doc.get("strings") or []}
    for row in rows:
        current = by_key.get(row["key"])
        if current is None:
            doc["strings"].append(row)
            by_key[row["key"]] = row
            continue
        current.update(row)
    write_gz(path, doc)


def update_source() -> None:
    path = ROOT / "SOURCE.json"
    manifest = json.loads(path.read_text(encoding="utf-8"))
    packs = manifest.setdefault("packs", {})
    for platform in ("android", "ios"):
        for file in sorted((ROOT / platform).glob("*.json.gz")):
            code = file.name[: -len(".json.gz")]
            doc = load_doc(file)
            entry = packs.get(f"{platform}/{code}") or {}
            entry["strings"] = len(doc["strings"])
            entry["bytes_gz"] = file.stat().st_size
            entry["version"] = doc.get("version")
            packs[f"{platform}/{code}"] = entry
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    manifest = json.loads((ROOT / "tellomi-client-keys.json").read_text(encoding="utf-8"))
    translator = Translator(Path("/tmp/tellomi-tellomi-mt.jsonl"), workers=6)
    engine = Engine(translator)
    pairs: list[tuple[str, str]] = []
    for platform in ("android", "ios"):
        keys = manifest_keys(manifest, platform)
        en_doc = load_doc(ROOT / platform / "en.json.gz")
        en_items = {item["key"]: item for item in en_doc["strings"]}
        for code in sorted(IOS_BASE):
            if code == "en" or code not in GOOGLE:
                continue
            doc = load_doc(ROOT / platform / f"{code}.json.gz")
            current = {item["key"]: text_of(item) for item in doc["strings"]}
            for key in keys:
                english = text_of(en_items.get(key))
                if not english or current.get(key) != english:
                    continue
                if english in BRANDS or english == "Moments" or key in CITIES:
                    continue
                source = GLOSS_OF_KEY.get((platform, key), english)
                if key == "Moments_DetailTitle":
                    continue
                shielded, _moments = protect_product(source)
                shielded, _ats = shield_ats(shielded)
                for piece in _pieces(shielded):
                    pairs.append((GOOGLE[code], piece))
    engine.translator.translate_all(pairs)
    allow_all: list[dict] = []
    summaries: list[tuple[str, str, dict[str, int]]] = []
    pending: list[tuple[str, str, dict, dict[str, tuple[str, int, str]]]] = []
    for platform in ("android", "ios"):
        keys = manifest_keys(manifest, platform)
        en_doc = load_doc(ROOT / platform / "en.json.gz")
        en_items = {item["key"]: item for item in en_doc["strings"]}
        for code in sorted(IOS_BASE):
            official = load_doc(ROOT / platform / f"{code}.json.gz")
            official_items = {item["key"]: item for item in official["strings"]}
            doc, allow_rows, counts = apply_language(engine, platform, code, keys, en_items, official_items)
            allow_all.extend(allow_rows)
            summaries.append((platform, code, counts))
            changed = {}
            by_new = {item["key"]: item for item in doc["strings"]}
            by_old = official_items
            version = int(doc["version"])
            for key in keys:
                new_item = by_new.get(key) or {}
                old_item = by_old.get(key) or {}
                if text_of(new_item) != text_of(old_item):
                    changed[key] = (text_of(new_item), version, new_item.get("mt_via") or "google")
            pending.append((platform, code, doc, changed))
    if engine.problems:
        print(f"{len(engine.problems)} strings still need a translation", file=sys.stderr)
        for line in engine.problems[:80]:
            print(line, file=sys.stderr)
        return 1
    for platform, code, doc, changed in pending:
        write_gz(ROOT / platform / f"{code}.json.gz", doc)
        if changed:
            update_reviewed(platform, code, changed)
            rows = []
            for key, (value, version, via) in sorted(changed.items()):
                rows.append({"key": key, "version": version, "value": value, "mt": True, "mt_via": via})
            update_machine(platform, code, rows)
    allow_all.sort(key=lambda row: (row["platform"], row["code"], row["key"]))
    allow_doc = {
        "note": "Tellomi values that are intentionally the same as English. "
        "brands match the whole string (PDF, Tellomi). entries are proper nouns, "
        "the product name when it is the same word, and labels this pack already "
        "writes in English (the official string for that word is English too). "
        "scripts/langpack_gap.py fails when any other Tellomi value equals English.",
        "brands": list(BRANDS),
        "entries": allow_all,
    }
    (ROOT / ALLOWLIST_NAME).write_text(json.dumps(allow_doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    update_source()
    print("| platform | code | already translated | newly translated | still English (allowlisted) | google | glossary | plural | product | city | sibling |")
    print("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
    for platform, code, counts in summaries:
        if code == "en":
            continue
        print(
            f"| {platform} | {code} | {counts['already']} | {counts['changed']} | {counts['allow']} | "
            f"{counts['google']} | {counts['glossary']} | {counts['plural']} | {counts['product']} | {counts['city']} | {counts['sibling']} |"
        )
    print(f"allowlist entries {len(allow_all)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
