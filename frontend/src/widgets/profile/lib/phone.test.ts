import { describe, expect, it } from "vitest";
import {
  formatLocal, hasIllegalChars, isPhoneValid, normalizePhone, phoneDigits, phoneErrorMessage,
  PHONE_ERROR_CHARS, PHONE_ERROR_LENGTH, PHONE_ERROR_OPERATOR, PHONE_HINT,
} from "./phone";

describe("phone", () => {
  it("достаёт 10 цифр и нормализует мобильный", () => {
    expect(phoneDigits("+7 912 345-67-89")).toBe("9123456789");
    expect(normalizePhone("9123456789")).toBe("+7 912 345-67-89");
    expect(formatLocal("9123456789")).toBe("912 345-67-89");
    expect(isPhoneValid("9123456789")).toBe(true);
  });

  it("ловит недопустимые символы, длину и оператора", () => {
    expect(hasIllegalChars("912abc")).toBe(true);
    expect(phoneErrorMessage("123", "912abc")).toBe(PHONE_ERROR_CHARS);
    expect(phoneErrorMessage("123")).toBe(PHONE_ERROR_LENGTH);
    expect(phoneErrorMessage("4123456789")).toBe(PHONE_ERROR_OPERATOR);
    expect(phoneErrorMessage("")).toBe("");
  });

  it("подсказка для пользователя задана", () => {
    expect(PHONE_HINT).toMatch(/Введите корректный номер телефона например/);
  });
});
