"use client";

import { useCallback } from "react";
import type { Connection } from "@xyflow/react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";
import { useCurrentOrg } from "@/stores/auth";
import { useMeshStore } from "@/stores/mesh";
import { requestBindingConnect } from "@/lib/api/connect/bindingConnect";
import { getLocalizedErrorMessage } from "@/lib/api/errors";

// Dragging between pod handles requests a PodBinding. React Flow node ids are
// pod keys (mesh-layout.ts), so source/target are the initiator/target pods.
// The backend rejects empty scopes, so grant read+write; `same_user_auto`
// activates the binding immediately for the caller's own pods and otherwise
// leaves it pending until the target pod accepts.
const POD_BINDING_SCOPES = ["pod:read", "pod:write"];
const AUTO_APPROVE_POLICY = "same_user_auto";

export function useMeshConnect() {
  const t = useTranslations();
  const orgSlug = useCurrentOrg()?.slug;
  const fetchTopology = useMeshStore((s) => s.fetchTopology);

  return useCallback(
    async (c: Connection) => {
      if (!orgSlug || !c.source || !c.target || c.source === c.target) return;
      try {
        await requestBindingConnect(orgSlug, c.source, c.target, POD_BINDING_SCOPES, AUTO_APPROVE_POLICY);
        fetchTopology();
        toast.success(t("mesh.connect.success"));
      } catch (err) {
        toast.error(getLocalizedErrorMessage(err, t, t("mesh.connect.error")));
      }
    },
    [orgSlug, fetchTopology, t],
  );
}
