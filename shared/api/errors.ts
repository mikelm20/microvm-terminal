import { z } from "zod";

export const ApiErrorCode = z.enum([
  "capacity_full",
  "auth_required",
  "auth_invalid",
  "magic_link_expired",
  "vm_launch_failed",
  "vm_not_found",
  "session_reaped",
  "bad_request",
  "not_found",
  "rate_limited",
  "internal",
]);
export type ApiErrorCode = z.infer<typeof ApiErrorCode>;

export const ApiError = z.object({
  code: ApiErrorCode,
  message: z.string(),
  retry_after_seconds: z.number().int().optional(),
  queue_position: z.number().int().optional(),
  request_id: z.string(),
});
export type ApiError = z.infer<typeof ApiError>;
