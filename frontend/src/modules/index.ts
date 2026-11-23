// Feature modules included in this build, in navigation order.
// Removing a module = deleting its line here and its folder.
import { finance } from './finance';
import type { AppModule } from './types';

export const modules: AppModule[] = [finance];

/** Where a brand-new account lands first: the first module that asks, else /app. */
export const signupPath: string = modules.find((m) => m.signupPath)?.signupPath ?? '/app';
