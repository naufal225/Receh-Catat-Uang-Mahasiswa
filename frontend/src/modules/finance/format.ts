// Finance-specific labels.
import type { Frequency, WalletType } from './types';

export const frequencyLabel: Record<Frequency, string> = {
  daily: 'Setiap hari',
  weekly: 'Setiap minggu',
  monthly: 'Setiap bulan',
  yearly: 'Setiap tahun',
};

export const walletTypeLabel: Record<WalletType, string> = {
  bank: 'Rekening bank',
  ewallet: 'E-wallet',
  cash: 'Tunai',
  other: 'Lainnya',
};
