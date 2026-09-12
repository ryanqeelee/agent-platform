/** True when nginx's skill-upload location (256MB) applies, not the knowledge 50MB cap. */
export function isSkillBundleUploadUrl(url: string | undefined): boolean {
  if (!url) return false
  const path = url.split('?')[0].replace(/\/+$/, '')
  return /(?:^|\/)api\/v1\/system\/admin\/tenants\/\d+\/skills\/catalog$/.test(path)
    || /(?:^|\/)api\/v1\/system\/admin\/tenants\/\d+\/sandbox-configs\/[^/]+\/skills$/.test(path)
}
