// Выбор коллеги: «Не ко мне» (BR-17), «Переадресовать» (BR-16), «Назначить» открытый вопрос (BR-37).
import { useState } from "react";
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, MenuItem, TextField, Typography } from "@mui/material";
import type { User } from "../api";

export const PersonDialog = ({ open, title, text, people, allowAuto, confirm, onClose, onPick }: {
  open: boolean; title: string; text: string; people: User[]; allowAuto?: boolean; confirm: string;
  onClose: () => void; onPick: (userId: string) => void;
}) => {
  const [who, setWho] = useState("");
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>{text}</Typography>
        <TextField select fullWidth label="Кому" value={who} onChange={(e) => setWho(e.target.value)}>
          {allowAuto && <MenuItem value="auto">Не знаю — пусть Напарник подберёт</MenuItem>}
          {people.map((u) => <MenuItem key={u.id} value={u.id}>{u.name} · {u.position}</MenuItem>)}
        </TextField>
      </DialogContent>
      <DialogActions sx={{ p: 2, gap: 1 }}>
        <Button variant="text" onClick={onClose}>Отмена</Button>
        <Button variant="contained" disabled={!who} onClick={() => { onPick(who === "auto" ? "" : who); setWho(""); }}>{confirm}</Button>
      </DialogActions>
    </Dialog>
  );
};
