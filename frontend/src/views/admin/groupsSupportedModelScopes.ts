export const normalizeSupportedModelScopesForPlatform = (
  platform: string,
  _scopes: string[] | undefined,
): string[] => {
  if (platform !== "antigravity") return [];
  return ["gemini_text"];
};
