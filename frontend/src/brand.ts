// Branding in one place. Colours are the tokens at the top of styles.css
// (--primary, --accent, ...); the logo mark is <Logo> in components/Icon.tsx.
// Keep `name` equal to APP_NAME in backend/.env.
export const brand = {
  name: 'Receh',
  /** Browser title when no page title applies, and the landing headline tag. */
  tagline: 'Keuangan mahasiswa',
  /** Prefix for localStorage keys, so two apps on one domain do not collide. */
  storagePrefix: 'receh',
};

/** Picks the label for the UI language (Indonesian for now; see Phase E). */
export function pick(label: Record<string, string> | undefined, lang = 'id'): string {
  if (!label) return '';
  return label[lang] ?? label.en ?? Object.values(label)[0] ?? '';
}
