// Карточка сотрудника (SCR-I5a, US-38): фото, ФИО, статус, должность, отдел, рабочие контакты.
// Своя карточка — загрузить или убрать фото, поправить телефон. ФИО, отдел и почта — из оргструктуры.
// Стиль — как в остальном интерфейсе: плоско, серые плашки, строки Row, без теней и скруглений (кроме фото-круга).
import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  Alert, Avatar, Box, Button, Dialog, DialogActions, DialogContent, IconButton, Link, Skeleton, Stack, TextField, Typography,
  useMediaQuery, useTheme,
} from "@mui/material";
import { CloseOutlined } from "@mui/icons-material";
import { deletePhoto, fetchPhoto, getProfile, MAX_PHOTO_BYTES, updatePhone, uploadPhoto, type Profile } from "@/shared/api";
import { StatusTag } from "@/shared/ui";

const PHOTO_TYPES = ["image/jpeg", "image/png", "image/webp"];

/** Object URL фото с заголовком X-User; освобождается при смене фото и размонтировании. */
export const usePhotoUrl = (login: string, url?: string | null) => {
  const [src, setSrc] = useState<string>();
  useEffect(() => {
    setSrc(undefined);
    if (!url) return;
    const controller = new AbortController();
    let created: string | undefined;
    fetchPhoto(login, url, controller.signal).then((u) => { created = u; if (!controller.signal.aborted) setSrc(u); else URL.revokeObjectURL(u); }).catch(() => undefined);
    return () => { controller.abort(); if (created) URL.revokeObjectURL(created); };
  }, [login, url]);
  return src;
};

/** Круг с фото или инициалами — заглушка, пока фото нет или оно грузится. */
export const ProfileAvatar = ({ login, photoUrl, initials, size = 32 }: { login: string; photoUrl?: string | null; initials: string; size?: number }) => {
  const { t } = useTheme();
  const src = usePhotoUrl(login, photoUrl);
  return <Avatar src={src} alt="" sx={{ width: size, height: size, fontSize: Math.round(size * 0.4), bgcolor: t.todaySoft, color: t.text }}>{initials}</Avatar>;
};

/** Уменьшает картинку до лимита бэкенда (512 КБ): длинная сторона до 1024 px, JPEG. Маленькие файлы — как есть. */
export const shrinkImage = async (file: File): Promise<Blob> => {
  if (!PHOTO_TYPES.includes(file.type)) throw new Error("Фото — JPEG, PNG или WebP.");
  if (file.size <= MAX_PHOTO_BYTES) return file;
  const bitmap = await createImageBitmap(file);
  for (const side of [1024, 768, 512]) {
    const scale = Math.min(1, side / Math.max(bitmap.width, bitmap.height));
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    canvas.getContext("2d")?.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob | null>((ok) => canvas.toBlob(ok, "image/jpeg", 0.85));
    if (blob && blob.size <= MAX_PHOTO_BYTES) return blob;
  }
  throw new Error("Не получилось уменьшить фото до 512 КБ — выбери другое.");
};

const dateRu = (iso: string) => new Date(`${iso}T00:00:00`).toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" });

const Field = ({ label, children }: { label: string; children: ReactNode }) => {
  const { t } = useTheme();
  return (
    <Stack direction={{ xs: "column", sm: "row" }} gap={{ xs: 0.25, sm: 2 }} sx={{ py: 1.5, borderBottom: `1px solid ${t.line}`, "&:last-child": { borderBottom: "none" } }}>
      <Typography variant="body2" color="text.secondary" sx={{ width: { sm: 140 }, flex: "none", pt: { sm: 0.25 } }}>{label}</Typography>
      <Box sx={{ minWidth: 0, flex: 1, overflowWrap: "anywhere" }}>{children}</Box>
    </Stack>
  );
};

export type ProfileDialogProps = {
  open: boolean;
  login: string;
  /** ID сотрудника; без него — своя карточка. */
  userId?: string;
  onClose: () => void;
  /** Своя карточка изменилась (фото, телефон) — например, обновить аватар в шапке. */
  onChanged?: (p: Profile) => void;
};

export const ProfileDialog = ({ open, login, userId, onClose, onChanged }: ProfileDialogProps) => {
  const theme = useTheme();
  const { t } = theme;
  const mobile = useMediaQuery(theme.breakpoints.down("sm"));
  const [profile, setProfile] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [phone, setPhone] = useState<string | null>(null); // null — не редактируем
  const [phoneError, setPhoneError] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const photo = usePhotoUrl(login, profile?.photo?.url);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setProfile(null); setError(""); setPhone(null); setPhoneError("");
    getProfile(login, userId, { signal: controller.signal })
      .then((p) => { if (!controller.signal.aborted) setProfile(p); })
      .catch((e) => { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : "Профиль недоступен"); });
    return () => controller.abort();
  }, [open, login, userId]);

  const apply = async (run: () => Promise<Profile>) => {
    setBusy(true); setError("");
    try { const p = await run(); setProfile(p); onChanged?.(p); return true; }
    catch (e) { setError(e instanceof Error ? e.message : "Не получилось"); return false; }
    finally { setBusy(false); }
  };

  const pickPhoto = async (f?: File) => {
    if (file.current) file.current.value = "";
    if (!f) return;
    await apply(async () => uploadPhoto(login, await shrinkImage(f)));
  };

  const savePhone = async () => {
    if (phone === null) return;
    setPhoneError("");
    setBusy(true);
    try { const p = await updatePhone(login, phone); setProfile(p); onChanged?.(p); setPhone(null); }
    catch (e) { setPhoneError(e instanceof Error ? e.message : "Не получилось сохранить"); }
    finally { setBusy(false); }
  };

  const intern = profile?.status === "Стажёр";

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" fullScreen={mobile} aria-labelledby="profile-title">
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ px: 3, pt: 2 }}>
        <Typography variant="caption" color="text.secondary">{profile?.can_edit ? "Мой профиль" : "Профиль сотрудника"}</Typography>
        <IconButton aria-label="Закрыть" onClick={onClose} edge="end"><CloseOutlined /></IconButton>
      </Stack>
      <DialogContent sx={{ pt: 1 }}>
        {error && <Alert severity="error" sx={{ mb: 2, borderRadius: 0 }}>{error}</Alert>}
        {!profile ? (!error && <Stack direction="row" gap={2.5} alignItems="center" aria-label="Загрузка профиля">
          <Skeleton variant="circular" width={96} height={96} />
          <Box flex={1}><Skeleton width="70%" height={32} /><Skeleton width="40%" /></Box>
        </Stack>) : <>
          <Stack direction={{ xs: "column", sm: "row" }} gap={2.5} alignItems={{ xs: "flex-start", sm: "center" }} sx={{ mb: 3 }}>
            <Avatar src={photo} alt={profile.photo ? `Фото: ${profile.name}` : ""} sx={{ width: 96, height: 96, fontSize: 34, bgcolor: t.todaySoft, color: t.text }}>{profile.initials}</Avatar>
            <Box minWidth={0}>
              <Typography id="profile-title" variant="h2" component="h2" sx={{ textWrap: "balance" }}>{profile.name}</Typography>
              <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mt: 1 }}>
                <StatusTag status={intern ? "Today" : "neutral"} label={profile.status} />
                <Typography variant="body2" color="text.secondary">{profile.position}</Typography>
              </Stack>
              {profile.can_edit && <Stack direction="row" gap={1} sx={{ mt: 2 }} flexWrap="wrap">
                <Button variant="outlined" size="small" disabled={busy} onClick={() => file.current?.click()}>{profile.photo ? "Сменить фото" : "Загрузить фото"}</Button>
                {profile.photo && <Button variant="text" size="small" disabled={busy} onClick={() => void apply(() => deletePhoto(login))}>Убрать фото</Button>}
                <input ref={file} type="file" hidden accept={PHOTO_TYPES.join(",")} data-testid="photo-input" onChange={(e) => void pickPhoto(e.target.files?.[0])} />
              </Stack>}
            </Box>
          </Stack>

          <Box sx={{ bgcolor: t.surface, px: { xs: 2, md: 3 }, py: 1 }}>
            <Field label="Отдел">{profile.department}</Field>
            <Field label="Телефон">
              {phone === null ? <Stack direction="row" gap={1.5} alignItems="baseline" flexWrap="wrap">
                {profile.phone ? <Link href={`tel:${profile.phone.replace(/[^\d+]/g, "")}`} color="inherit">{profile.phone}</Link> : <Typography color="text.secondary">не указан</Typography>}
                {profile.can_edit && <Link component="button" variant="body2" onClick={() => setPhone(profile.phone)} sx={{ color: t.link }}>Изменить</Link>}
              </Stack> : <Stack gap={1.5}>
                <TextField size="small" fullWidth autoFocus label="Рабочий телефон" value={phone} onChange={(e) => setPhone(e.target.value)} error={Boolean(phoneError)}
                  helperText={phoneError || "Например, +7 900 123-45-67"} inputProps={{ inputMode: "tel" }}
                  onFocus={(e) => e.target.select()}
                  onKeyDown={(e) => { if (e.key === "Enter") void savePhone(); if (e.key === "Escape") { e.stopPropagation(); setPhone(null); setPhoneError(""); } }} />
                <Stack direction="row" gap={1}>
                  <Button variant="contained" size="small" disabled={busy || !phone.trim()} onClick={() => void savePhone()}>Сохранить</Button>
                  <Button variant="text" size="small" disabled={busy} onClick={() => { setPhone(null); setPhoneError(""); }}>Отмена</Button>
                </Stack>
              </Stack>}
            </Field>
            <Field label="Почта">{profile.email ? <Link href={`mailto:${profile.email}`} color="inherit">{profile.email}</Link> : <Typography color="text.secondary">не указана</Typography>}</Field>
            {intern && <>
              {profile.start_date && <Field label="Старт стажировки">{dateRu(profile.start_date)}{profile.week ? ` · ${profile.week}-я неделя` : ""}</Field>}
              {profile.buddy && <Field label="Бадди">{profile.buddy}</Field>}
              {profile.lead && <Field label="Руководитель">{profile.lead}</Field>}
            </>}
          </Box>
          {profile.can_edit && <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1.5 }}>
            ФИО, должность, отдел и почта — из оргструктуры. Фото — JPEG, PNG или WebP; большое уменьшим сами.
          </Typography>}
        </>}
      </DialogContent>
      {mobile && <DialogActions sx={{ p: 2 }}><Button variant="contained" fullWidth onClick={onClose}>Готово</Button></DialogActions>}
    </Dialog>
  );
};
