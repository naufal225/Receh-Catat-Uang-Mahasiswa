// The contract between the app shell and a feature module. A module lists its
// pages, its navigation and, optionally, a provider for shared state, an
// overlay (global modals) and a primary action (the big "+" button).
import type { ComponentType, ReactNode } from 'react';
import type { IconName } from '../components/Icon';

export interface ModulePage {
  el: ComponentType;
  /** Browser tab title. */
  title: string;
}

export interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  /** Shown in the mobile bottom bar with this short label (max ~4 items in total). */
  short?: string;
}

export interface PrimaryAction {
  label: string;
  short: string;
  icon: IconName;
  /** Keyboard shortcut (single key), e.g. "n". */
  shortcut?: string;
  /** Hook returning what the button does; runs inside the module provider. */
  use: () => { run: () => void; disabled?: boolean };
}

export interface AppModule {
  id: string;
  /** User pages by path, e.g. "/app/dompet". "/app" is the home page. */
  pages: Record<string, ModulePage>;
  nav: NavItem[];
  Provider?: ComponentType<{ children: ReactNode }>;
  Overlay?: ComponentType;
  primary?: PrimaryAction;
}
