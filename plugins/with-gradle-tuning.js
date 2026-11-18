const { withGradleProperties } = require('expo/config-plugins');

/** Dev-machine Gradle settings, driven by env so EAS builds keep the defaults (all ABIs, larger heap). */
module.exports = function withGradleTuning(config) {
  const abis = process.env.FIXR_ANDROID_ABIS;
  const heap = process.env.FIXR_GRADLE_HEAP;
  if (!abis && !heap) return config;
  return withGradleProperties(config, (cfg) => {
    const set = (key, value) => {
      const existing = cfg.modResults.find((p) => p.type === 'property' && p.key === key);
      if (existing) existing.value = value;
      else cfg.modResults.push({ type: 'property', key, value });
    };
    if (abis) set('reactNativeArchitectures', abis);
    if (heap) {
      set('org.gradle.jvmargs', `-Xmx${heap} -XX:MaxMetaspaceSize=512m`);
      set('org.gradle.workers.max', '3');
      set('kotlin.daemon.jvmargs', '-Xmx768m');
    }
    return cfg;
  });
};
