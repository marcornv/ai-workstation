// i18n.ts: the interface language (Portuguese or English).
//
// The language is the "language" setting in bancada.yaml. The Go server writes
// it into the page as <html lang="...">, so it is known before anything renders.
// Changing it reloads the page. Texts are written in place with both versions:
// t("Salvar", "Save").

export type Language = "pt-BR" | "en";

const current: Language = document.documentElement.lang === "en" ? "en" : "pt-BR";

export const language = () => current;

// t returns the text in the current language.
export const t = (pt: string, en: string) => (current === "en" ? en : pt);

// Locale for numbers and dates: "1.234,5" in Portuguese, "1,234.5" in English.
export const locale = () => (current === "en" ? "en-US" : "pt-BR");
