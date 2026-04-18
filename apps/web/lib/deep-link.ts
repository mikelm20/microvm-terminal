export const APP_SCHEME = "learn";
export const UNIVERSAL_HOST = "learn.example.com";
export const APP_STORE_URL = "https://apps.apple.com/app/idPLACEHOLDER";
export const PLAY_STORE_URL = "https://play.google.com/store/apps/details?id=com.example.learn";

/**
 * Returns a URL that opens the native app if installed and falls back to the
 * web page otherwise. Browsers handle `learn://` schemes inconsistently; a
 * universal link (https) is usually more reliable. We stay flexible here.
 */
export function appDeepLink(pathInsideApp: string): string {
  const safe = pathInsideApp.startsWith("/") ? pathInsideApp : `/${pathInsideApp}`;
  return `${APP_SCHEME}://${safe.slice(1)}`;
}

export function universalDeepLink(pathInsideApp: string): string {
  const safe = pathInsideApp.startsWith("/") ? pathInsideApp : `/${pathInsideApp}`;
  return `https://${UNIVERSAL_HOST}${safe}`;
}

export function linkedinAddCertificateUrl(params: {
  name: string;
  issueDate: Date;
  certificateId: string;
  certificateUrl: string;
}): string {
  const q = new URLSearchParams({
    startTask: "CERTIFICATION_NAME",
    name: params.name,
    organizationName: "Platform Engineering",
    issueYear: String(params.issueDate.getUTCFullYear()),
    issueMonth: String(params.issueDate.getUTCMonth() + 1),
    certId: params.certificateId,
    certUrl: params.certificateUrl,
  });
  return `https://www.linkedin.com/profile/add?${q.toString()}`;
}
