"use client";

import { useState, useRef, useId } from "react";
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
  const { branches, fallbackToFreeText, load } = useRepositoryBranches(repoId);
  const [open, setOpen] = useState(false);
  const listboxId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  if (fallbackToFreeText) {
    return <BranchInput value={value} onChange={onChange} error={error} t={t} />;
  }

  const filtered = value
    ? branches.filter((b) => b.toLowerCase().includes(value.toLowerCase()))
    : branches;

  function handleFocus() {
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
          aria-controls={open ? `${listboxId}-listbox` : undefined}
          aria-invalid={!!error}
          aria-describedby={error ? `${listboxId}-error` : undefined}
          type="text"
          className={`w-full px-3 py-2 border rounded-md bg-background ${
            error ? "border-destructive" : "border-border"
          }`}
          placeholder={t("ide.createPod.branchPlaceholder")}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={handleFocus}
        />
        {open && filtered.length > 0 && (
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
                onClick={() => selectBranch(branch)}
              >
                {branch}
              </li>
            ))}
          </ul>
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
