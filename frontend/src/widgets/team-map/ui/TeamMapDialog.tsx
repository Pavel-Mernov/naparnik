// Карта команды (SCR-I5, US-06, SC-02): кто за что отвечает, их страницы в базе и черновик первого сообщения.
// Напарник ничего не отправляет коллегам: человек копирует черновик и пишет сам (BR-62).
import { useEffect, useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import { api, type TeamCard } from "@/shared/api";
import { AiTag, DialogClose, dialogTitleSx, Empty, ListSkeleton, StatusTag } from "@/shared/ui";
import { ProfileAvatar } from "@/widgets/profile";

const Card = ({ person, login, onOpenPage, onProfile }: {
  person: TeamCard; login: string; onOpenPage: (id: string) => void; onProfile?: (id: string) => void;
}) => {
  const { t } = useTheme();
  const [writing, setWriting] = useState(false);
  const [text, setText] = useState(person.message);
  const [copied, setCopied] = useState("");
  const copy = async () => {
    try { await navigator.clipboard.writeText(text); setCopied("Скопировано — вставь в мессенджер и отправь сам"); }
    catch { setCopied("Не получилось скопировать — выдели текст и скопируй вручную"); }
  };
  return (
    <Box component="article" aria-label={person.name} sx={{ bgcolor: t.surface, p: 2.5, display: "flex", flexDirection: "column", gap: 1.25 }}>
      <Stack direction="row" gap={1.5} alignItems="center">
        <Box component="button" onClick={() => onProfile?.(person.id)} aria-label={`Профиль: ${person.name}`} disabled={!onProfile}
          sx={{ p: 0, border: "none", bgcolor: "transparent", cursor: onProfile ? "pointer" : "default", lineHeight: 0 }}>
          <ProfileAvatar login={login} photoUrl={person.photo?.url ?? null} initials={person.initials} size={48} />
        </Box>
        <Box minWidth={0} flex={1}>
          <Typography fontWeight={600}>{person.name}</Typography>
          <Typography variant="body2" color="text.secondary">{person.position}</Typography>
        </Box>
        {person.my_buddy ? <StatusTag status="Today" label="твой бадди" /> : <StatusTag status="Planned" label={person.status} />}
      </Stack>
      {person.responsibility && <Typography variant="body2"><Box component="span" color="text.secondary">Отвечает за: </Box>{person.responsibility}</Typography>}
      {person.working_hours && <Typography variant="body2" color="text.secondary">Работает {person.working_hours}</Typography>}
      {person.pages.length > 0 && <Box>
        <Typography variant="body2" color="text.secondary" mb={0.5}>Пишет в базе:</Typography>
        <Stack gap={0.25} alignItems="flex-start">
          {person.pages.map((p) => <Button key={p.id} variant="text" size="small" onClick={() => onOpenPage(p.id)}
            sx={{ p: 0, minHeight: 28, textAlign: "left", justifyContent: "flex-start", bgcolor: "transparent", textDecoration: "underline", "&:hover": { bgcolor: "transparent" } }}>{p.title}</Button>)}
        </Stack>
      </Box>}
      {person.via_buddy ? <Typography variant="body2" color="text.secondary" sx={{ mt: "auto" }}>Вопросы по этим темам — через бадди.</Typography>
        : !writing ? <Button variant="outlined" size="small" sx={{ alignSelf: "flex-start", mt: "auto" }} onClick={() => setWriting(true)}>Написать</Button> : <Box sx={{ mt: 0.5 }}>
        <Stack direction="row" alignItems="center" gap={1} mb={0.75}>
          <Typography component="label" htmlFor={`msg-${person.id}`} variant="body2" fontWeight={600}>Черновик сообщения</Typography>
          <AiTag>Напарник</AiTag>
        </Stack>
        <TextField id={`msg-${person.id}`} fullWidth multiline minRows={3} value={text} onChange={(e) => { setText(e.target.value); setCopied(""); }}
          sx={{ "& .MuiInputBase-root": { bgcolor: t.bg } }} />
        <Stack direction="row" gap={1} mt={1} alignItems="center" flexWrap="wrap">
          <Button variant="contained" size="small" disabled={!text.trim()} onClick={() => void copy()}>Скопировать</Button>
          <Button variant="text" size="small" onClick={() => { setWriting(false); setCopied(""); setText(person.message); }}>Свернуть</Button>
        </Stack>
        <Typography variant="caption" color="text.secondary" role="status" sx={{ display: "block", mt: 0.75 }}>
          {copied || "Напарник ничего не отправит сам — сообщение пишешь ты."}
        </Typography>
      </Box>}
    </Box>
  );
};

export const TeamMapDialog = ({ open, login, onClose, onOpenPage, onProfile }: {
  open: boolean; login: string; onClose: () => void; onOpenPage: (id: string) => void; onProfile?: (id: string) => void;
}) => {
  const mobile = useMediaQuery(useTheme().breakpoints.down("sm"));
  const [people, setPeople] = useState<TeamCard[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    const c = new AbortController();
    setPeople(null); setError("");
    api<{ people: TeamCard[] }>(login, "/api/team", { signal: c.signal })
      .then((r) => setPeople(r.people))
      .catch((e) => { if (!c.signal.aborted) setError(e instanceof Error ? e.message : "Не удалось загрузить команду"); });
    return () => c.abort();
  }, [open, login]);
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md" fullScreen={mobile}>
      <DialogTitle sx={{ ...dialogTitleSx, pb: 0.5 }}>
        <Box component="span" sx={{ flex: 1, minWidth: 0, pr: 1 }}>Карта команды</Box>
        <DialogClose onClose={onClose} />
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" mb={2}>Кто за что отвечает и с какими вопросами к кому идти. «Написать» — черновик знакомства, который ты отправишь сам.</Typography>
        {error ? <Typography color="error.main" role="alert">{error}</Typography>
          : !people ? <ListSkeleton plates={1} rows={3} />
          : people.length ? <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)" }, gap: 1.5 }}>
            {people.map((p) => <Card key={p.id} person={p} login={login} onOpenPage={onOpenPage} onProfile={onProfile} />)}
          </Box>
          : <Empty text="В отделе пока никого нет в справочнике." variant={1} />}
      </DialogContent>
      <DialogActions sx={{ p: 2 }}>
        <Button variant="text" onClick={onClose}>Закрыть</Button>
      </DialogActions>
    </Dialog>
  );
};
