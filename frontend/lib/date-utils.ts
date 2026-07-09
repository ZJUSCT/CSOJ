import { format } from 'date-fns';
import { zhCN, enUS, Locale } from 'date-fns/locale';

const locales: Record<string, Locale> = {
    zh: zhCN,
    en: enUS,
};

// Get locale string from localStorage (set by i18n provider)
function getLocaleStr(): string {
    if (typeof window === 'undefined') return 'en';
    return localStorage.getItem('csoj_locale') || 'en';
}

// Format with locale-aware date+time
// zh: 2026年1月1日 08:00
// en: Jan 1, 2026 08:00
export function formatDateTime(date: Date | string | number, localeStr?: string): string {
    const d = typeof date === 'object' ? date : new Date(date);
    const loc = localeStr || getLocaleStr();
    const locale = locales[loc] || enUS;
    if (loc === 'zh') {
        return format(d, 'yyyy年M月d日 HH:mm', { locale });
    }
    return format(d, 'MMM d, yyyy HH:mm', { locale });
}

// Format date range
// zh: 2026年1月1日 08:00 - 2027年1月1日 08:00
// en: Jan 1, 2026 08:00 - Jan 1, 2027 08:00
export function formatDateRange(start: Date | string | number, end: Date | string | number, localeStr?: string): string {
    return formatDateTime(start, localeStr) + ' - ' + formatDateTime(end, localeStr);
}

// Short date+time for table cells
// zh: 2026/01/01 08:00
// en: 01/01/2026 08:00
export function formatShort(date: Date | string | number, localeStr?: string): string {
    const d = typeof date === 'object' ? date : new Date(date);
    const loc = localeStr || getLocaleStr();
    if (loc === 'zh') {
        return format(d, 'yyyy/MM/dd HH:mm');
    }
    return format(d, 'MM/dd/yyyy HH:mm');
}
