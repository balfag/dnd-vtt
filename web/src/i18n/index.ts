import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './en.json'
import ru from './ru.json'

export const defaultLanguage = 'ru'

export const resources = {
  ru: { translation: ru },
  en: { translation: en },
} as const

void i18n.use(initReactI18next).init({
  resources,
  lng: defaultLanguage,
  fallbackLng: defaultLanguage,
  // React already escapes interpolated strings.
  interpolation: { escapeValue: false },
})

export default i18n
