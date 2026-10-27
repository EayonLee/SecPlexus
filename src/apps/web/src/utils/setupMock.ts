export const mockEnabled =
  import.meta.env.DEV && import.meta.env.VITE_ENABLE_MOCK !== 'false';

export default (config: { mock?: boolean; setup: () => void }) => {
  if (mockEnabled && config.mock !== false) config.setup();
};
