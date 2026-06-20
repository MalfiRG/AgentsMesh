"use client";

import { useCallback } from "react";
import { Locale, locales } from "@/lib/i18n/config";
import { LOCALE_STORAGE_KEY, LOCALE_CHANGE_EVENT } from "../../providers/IntlProvider";

export function useSetLocale() {
  return useCallback((newLocale: Locale) => {
    if (!locales.includes(newLocale)) return;
    localStorage.setItem(LOCALE_STORAGE_KEY, newLocale);
    document.documentElement.lang = newLocale;
    window.dispatchEvent(new CustomEvent(LOCALE_CHANGE_EVENT, { detail: newLocale }));
  }, []);
}
