// Выбор коллеги: «Не ко мне» (BR-17), «Переадресовать» (BR-16), «Назначить» открытый вопрос (BR-37).
// Подпись «Кому» — над полем (рамка фокуса в теме без выреза под плавающую метку), список — ровно под полем
// и не шире его: имя и должность в две строки, ничего не обрезается.
import { useRef, useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, MenuItem, TextField, Typography, useTheme } from "@mui/material";
import type { User } from "@/shared/api";

const Person = ({ name, hint }: { name: string; hint?: string }) => (
  <Box sx={{ minWidth: 0, py: 0.25 }}>
    <Typography sx={{ whiteSpace: "normal", lineHeight: 1.3 }}>{name}</Typography>
    {hint && <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: "normal", lineHeight: 1.3 }}>{hint}</Typography>}
  </Box>
);

export const PersonDialog = ({ open, title, text, people, allowAuto, confirm, onClose, onPick }: {
  open: boolean; title: string; text: string; people: User[]; allowAuto?: boolean; confirm: string;
  onClose: () => void; onPick: (userId: string) => void;
}) => {
  const { t } = useTheme();
  const [who, setWho] = useState("");
  const field = useRef<HTMLDivElement>(null);
  // Список помещается под полем: высота — по свободному месту до низа окна, иначе меню наезжало бы на поле.
  const [menu, setMenu] = useState({ width: 0, maxHeight: 360 });
  const measure = () => {
    const r = field.current?.getBoundingClientRect();
    if (r) setMenu({ width: r.width, maxHeight: Math.max(160, Math.min(360, window.innerHeight - r.bottom - 24)) });
  };
  const label = (id: string) => (id === "auto" ? "Пусть Напарник подберёт" : people.find((u) => u.id === id)?.name ?? "");
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>{text}</Typography>
        <Typography component="label" id="person-label" htmlFor="person-select" variant="body2" fontWeight={600} sx={{ display: "block", mb: 0.75 }}>Кому</Typography>
        <TextField ref={field} id="person-select" select fullWidth value={who} onChange={(e) => setWho(e.target.value)}
          sx={{ "& .MuiInputBase-root": { bgcolor: t.bg } }}
          slotProps={{
            select: {
              displayEmpty: true,
              onOpen: measure,
              labelId: "person-label",
              renderValue: (v) => (v ? label(String(v)) : <Box component="span" sx={{ color: "text.secondary" }}>Выбери сотрудника</Box>),
              MenuProps: {
                anchorEl: () => field.current?.querySelector<HTMLElement>(".MuiInputBase-root") ?? field.current!, // вся рамка поля
                anchorOrigin: { vertical: "bottom", horizontal: "left" },
                transformOrigin: { vertical: "top", horizontal: "left" },
                slotProps: { paper: { sx: { width: menu.width || undefined, maxWidth: "calc(100vw - 32px)", maxHeight: menu.maxHeight, boxSizing: "border-box", mt: 0.5 } } },
              },
            },
          }}>
          {allowAuto && <MenuItem value="auto"><Person name="Не знаю — пусть Напарник подберёт" /></MenuItem>}
          {people.map((u) => <MenuItem key={u.id} value={u.id}><Person name={u.name} hint={u.position} /></MenuItem>)}
        </TextField>
      </DialogContent>
      <DialogActions sx={{ p: 2, gap: 1 }}>
        <Button variant="text" onClick={onClose}>Отмена</Button>
        <Button variant="contained" disabled={!who} onClick={() => { onPick(who === "auto" ? "" : who); setWho(""); }}>{confirm}</Button>
      </DialogActions>
    </Dialog>
  );
};
