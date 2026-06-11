"use client";

import { useEffect, useState, useCallback } from "react";
import { useTranslations } from "next-intl";
import { ChevronDown, Loader2, Check, FolderGit2 } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useRepositories, useRepositoryStore } from "@/stores/repository";
import { cn } from "@/lib/utils";

interface RepositoryInlineSelectProps {
  value: number | null;
  currentName?: string;
  onChange: (repositoryId: number | null) => Promise<void>;
  disabled?: boolean;
  size?: "sm" | "md";
}

const sizeClasses = { sm: "h-6 text-xs", md: "h-8 text-sm" };
const iconSizeClasses = { sm: "h-3.5 w-3.5", md: "h-4 w-4" };

export function RepositoryInlineSelect({
  value,
  currentName,
  onChange,
  disabled = false,
  size = "sm",
}: RepositoryInlineSelectProps) {
  const t = useTranslations();
  const repositories = useRepositories().filter((r) => r.is_active);
  const fetchRepositories = useRepositoryStore((s) => s.fetchRepositories);
  const [isOpen, setIsOpen] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);

  useEffect(() => { fetchRepositories(); }, [fetchRepositories]);

  const selected = repositories.find((r) => r.id === value);
  const label = selected?.name ?? currentName ?? t("tickets.detail.noRepository");

  const handleSelect = useCallback(async (repositoryId: number | null) => {
    if (repositoryId === value || disabled) return;
    setIsUpdating(true);
    setIsOpen(false);
    try {
      await onChange(repositoryId);
    } catch (error) {
      console.error("Failed to update repository:", error);
    } finally {
      setIsUpdating(false);
    }
  }, [value, onChange, disabled]);

  return (
    <DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
      <DropdownMenuTrigger
        disabled={disabled || isUpdating}
        className={cn(
          "inline-flex items-center gap-1.5 px-2 rounded-md transition-all",
          "hover:bg-muted focus:outline-none focus:ring-2 focus:ring-primary/20",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          sizeClasses[size],
        )}
      >
        {isUpdating ? (
          <Loader2 className={cn("animate-spin", iconSizeClasses[size], "text-muted-foreground")} />
        ) : (
          <FolderGit2 className={cn(iconSizeClasses[size], "text-muted-foreground")} />
        )}
        <span className="font-mono font-medium text-foreground">{label}</span>
        <ChevronDown className="h-3 w-3 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-64 w-56 overflow-y-auto">
        {repositories.map((repo) => {
          const isSelected = repo.id === value;
          return (
            <DropdownMenuItem
              key={repo.id}
              onClick={() => handleSelect(repo.id)}
              className={cn("flex items-center gap-2 cursor-pointer", isSelected && "bg-muted")}
            >
              <FolderGit2 className="h-4 w-4 text-muted-foreground" />
              <span className={cn("truncate font-mono", isSelected && "font-medium")}>{repo.name}</span>
              {isSelected && <Check className="ml-auto h-3 w-3 text-primary" />}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export default RepositoryInlineSelect;
