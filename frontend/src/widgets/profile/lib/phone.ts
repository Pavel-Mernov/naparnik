/** Сообщения валидации телефона (UX). */
export const PHONE_HINT = "Введите корректный номер телефона например,\n+7 (999) 123-45-67.";

export const PHONE_ERROR =
  "Неверный формат номера телефона. Пожалуйста, проверьте и попробуйте снова.";

export const PHONE_SUCCESS = "Номер телефона успешно валидирован. Спасибо!";

export const PHONE_ERROR_LENGTH =
  "Номер телефона должен содержать ровно 11 цифр";

export const PHONE_ERROR_CHARS =
  "Номер телефона может содержать только цифры, пробелы и символ +. Использование букв и других символов недопустимо.";

export const PHONE_ERROR_COUNTRY =
  "Введите код страны в соответствии с международными стандартами. Убедитесь, что код указан правильно.";

export const PHONE_ERROR_OPERATOR =
  "Номер должен быть действительным для вашего региона. Проверьте, чтобы код оператора был корректным.";

/** Только цифры локальной части (без +7 / 8), максимум 10. */
export const phoneDigits = (raw: string): string => {
  let d = raw.replace(/\D/g, "");
  if (d.length >= 11 && (d[0] === "7" || d[0] === "8")) d = d.slice(1);
  return d.slice(0, 10);
};

/** Отображение локальной части: 999 123-45-67 */
export const formatLocal = (digits: string): string => {
  const d = phoneDigits(digits);
  if (d.length <= 3) return d;
  if (d.length <= 6) return `${d.slice(0, 3)} ${d.slice(3)}`;
  if (d.length <= 8) return `${d.slice(0, 3)} ${d.slice(3, 6)}-${d.slice(6)}`;
  return `${d.slice(0, 3)} ${d.slice(3, 6)}-${d.slice(6, 8)}-${d.slice(8)}`;
};

/** Как backend: 10 цифр → «+7 900 123-45-67». */
export const normalizePhone = (raw: string): string | null => {
  const digits = phoneDigits(raw);
  if (digits.length !== 10 || digits[0] === "0") return null;
  // Российские мобильные: код оператора обычно 9xx
  if (digits[0] !== "9") return null;
  return `+7 ${digits.slice(0, 3)} ${digits.slice(3, 6)}-${digits.slice(6, 8)}-${digits.slice(8, 10)}`;
};

/** true, если в строке есть недопустимые символы (не цифра, не пробел, не +, -, (, )). */
export const hasIllegalChars = (raw: string): boolean => /[^\d\s+\-()]/.test(raw);

/**
 * Проверки по порядку: символы → длина → код страны (+7 фиксирован) → оператор → формат.
 * Пустая строка — без ошибки (показываем подсказку).
 */
export const phoneErrorMessage = (localDigits: string, rawInput?: string): string => {
  if (rawInput !== undefined && hasIllegalChars(rawInput)) return PHONE_ERROR_CHARS;
  const d = phoneDigits(localDigits);
  if (!d) return "";
  // 10 локальных + код страны 7 = 11 цифр
  if (d.length < 10) return PHONE_ERROR_LENGTH;
  if (d[0] === "0") return PHONE_ERROR_COUNTRY;
  if (d[0] !== "9") return PHONE_ERROR_OPERATOR;
  if (!normalizePhone(d)) return PHONE_ERROR;
  return "";
};

export const isPhoneValid = (localDigits: string) => Boolean(normalizePhone(localDigits));
