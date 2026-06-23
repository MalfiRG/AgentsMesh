"use client";

import { useState, useRef, useId } from "react";
import { ChevronDown, Loader2 } from "lucide-react";
import { useRepositoryBranches } from "@/hooks/useRepositoryBranches";
import { BranchInput } from "./RepositorySelect";

interface BranchComboboxProps {
  repoId: number;
  value: string;
  onChange: (value: string) => void;
  error?: string;
  t: (key: string) => string;
}

export function BranchCombobox({ repoId, value, onChange, error, t }: BranchComboboxProps) {
  const { branches, loading, fallbackToFreeText, load } = useRepositoryBranches(repoId);
  const [open, setOpen] = useState(false);
  const listboxId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  if (fallbackToFreeText) {
    return <BranchInput value={value} onChange={onChange} error={error} t={t} />;
  }

  const filtered = value
    ? branches.filter((b) => b.toLowerCase().includes(value.toLowerCase()))
    : branches;

  function openList() {
    load();
    setOpen(true);
  }

  function handleBlur(e: React.FocusEvent) {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      setOpen(false);
    }
  }

  function selectBranch(branch: string) {
    onChange(branch);
    setOpen(false);
    inputRef.current?.focus();
  }

  function toggle() {
    if (open) {
      setOpen(false);
    } else {
      openList();
      inputRef.current?.focus();
    }
  }

  const showList = open && !loading && filtered.length > 0;
  const showEmpty = open && !loading && filtered.length === 0;

  return (
    <div onBlur={handleBlur}>
      <label htmlFor={listboxId} className="block text-sm font-medium mb-2">
        {t("ide.createPod.branch")}
      </label>
      <div className="relative">
        <input
          ref={inputRef}
          id={listboxId}
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={open}
          aria-controls={showList ? `${listboxId}-listbox` : undefined}
          aria-invalid={!!error}
          aria-describedby={error ? `${listboxId}-error` : undefined}
          type="text"
          className={`w-full px-3 py-2 pr-9 border rounded-md bg-background ${
            error ? "border-destructive" : "border-border"
          }`}
          placeholder={t("ide.createPod.branchPlaceholder")}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={openList}
        />
        <button
          type="button"
          tabIndex={-1}
          aria-label={t("ide.createPod.branchToggle")}
          onClick={toggle}
          className="absolute inset-y-0 right-0 flex items-center px-2 text-muted-foreground hover:text-foreground"
        >
          {loading ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
          )}
        </button>
        {showList && (
          <ul
            id={`${listboxId}-listbox`}
            role="listbox"
            className="absolute z-50 w-full mt-1 bg-background border border-border rounded-md shadow-md max-h-48 overflow-y-auto"
          >
            {filtered.map((branch) => (
              <li
                key={branch}
                role="option"
                aria-selected={branch === value}
                className="px-3 py-2 cursor-pointer hover:bg-accent text-sm truncate"
                onPointerDown={(e) => {
                  e.preventDefault();
                  selectBranch(branch);
                }}
              >
                {branch}
              </li>
            ))}
          </ul>
        )}
        {showEmpty && (
          <div className="absolute z-50 w-full mt-1 bg-background border border-border rounded-md shadow-md px-3 py-2 text-sm text-muted-foreground">
            {t("ide.createPod.branchEmpty")}
          </div>
        )}
      </div>
      {error && (
        <p id={`${listboxId}-error`} className="text-xs text-destructive mt-1">
          {error}
        </p>
      )}
    </div>
  );
}
