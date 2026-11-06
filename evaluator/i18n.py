"""Interface language for the reasons and errors shown to the user.

Go sends "language" ("pt-BR" or "en") with every request; main.py stores it
here for the duration of that request. Messages are written as tr(pt, en).
"""

from contextvars import ContextVar

language: ContextVar[str] = ContextVar("language", default="pt-BR")


def tr(pt: str, en: str) -> str:
    return en if language.get() == "en" else pt
