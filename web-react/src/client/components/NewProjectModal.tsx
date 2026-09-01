import { useState, useEffect, useRef } from "react";
import { DEFAULT_NEW_PROJECT_ROOT } from "../../shared/branding";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogBody,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { SegmentedControl } from "./ui/segmented-control";
import { useI18n } from "../i18n/context";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (project: { mode: Tab; localPath: string; title: string }) => void;
}

type Tab = "new" | "existing";

function toKebab(str: string): string {
  return str
    .toLowerCase()
    .replace(/[\s_]+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function NewProjectModal({ open, onOpenChange, onConfirm }: Props) {
  const { t } = useI18n();
  const [tab, setTab] = useState<Tab>("new");
  const [name, setName] = useState("");
  const [existingPath, setExistingPath] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const existingInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setTab("new");
      setName("");
      setExistingPath("");
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  useEffect(() => {
    if (open) {
      setTimeout(() => {
        if (tab === "new") inputRef.current?.focus();
        else existingInputRef.current?.focus();
      }, 0);
    }
  }, [tab, open]);

  const kebab = toKebab(name);
  const newPath = kebab ? `${DEFAULT_NEW_PROJECT_ROOT}/${kebab}` : "";
  const trimmedExisting = existingPath.trim();

  const canSubmit = tab === "new" ? !!kebab : !!trimmedExisting;

  const handleSubmit = () => {
    if (!canSubmit) return;
    if (tab === "new") {
      onConfirm({ mode: "new", localPath: newPath, title: name.trim() });
    } else {
      const folderName = trimmedExisting.split("/").pop() || trimmedExisting;
      onConfirm({
        mode: "existing",
        localPath: trimmedExisting,
        title: folderName,
      });
    }
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm">
        <DialogBody className="space-y-4">
          <DialogTitle>{t.newProject.addProject}</DialogTitle>
          <DialogDescription className="sr-only">
            {t.newProject.addProject}
          </DialogDescription>

          <SegmentedControl
            value={tab}
            onValueChange={setTab}
            options={[
              { value: "new" as Tab, label: t.newProject.newFolder },
              { value: "existing" as Tab, label: t.newProject.existingPath },
            ]}
            className="w-full mb-2"
            optionClassName="flex-1 justify-center"
          />

          {tab === "new" ? (
            <div className="space-y-2">
              <Input
                ref={inputRef}
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSubmit();
                  if (e.key === "Escape") onOpenChange(false);
                }}
                placeholder={t.newProject.projectName}
              />
              {newPath && (
                <p className="text-xs text-muted-foreground font-mono">
                  {newPath}
                </p>
              )}
            </div>
          ) : (
            <div className="space-y-2">
              <Input
                ref={existingInputRef}
                type="text"
                value={existingPath}
                onChange={(e) => setExistingPath(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSubmit();
                  if (e.key === "Escape") onOpenChange(false);
                }}
                placeholder="~/Projects/my-app"
              />
              <p className="text-xs text-muted-foreground">
                {t.newProject.folderCreated}
              </p>
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            {t.common.cancel}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={handleSubmit}
            disabled={!canSubmit}
          >
            {t.common.create}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
