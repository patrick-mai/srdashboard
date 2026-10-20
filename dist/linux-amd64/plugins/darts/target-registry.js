/**
 * Darts face. Every discipline uses this one profile.
 * 1 SVG unit = 1 mm, centre 100,100. OpticScore stays 100 DSG/mm.
 */
(function () {
  const DSG_COORD_RANGE = 9000;

  const PROFILES = {
    dartboard: {
      id: 'dartboard',
      label: 'Dartscheibe',
      file: 'dartboard.svg',
      targetDiameterMm: 170.6,
      tenRingDiameterMm: 8.0,
      innerTenDiameterMm: 8.0,
      ringWidthMm: 4.0,
      oneRingSvg: 4.0,
      ring8RadiusMm: 85.3,
      shotDiameterMm: 2,
      decMax: 10.9,
      coordRadiusMm: 90,
      teilerBandDsg: 25,
      last10Max: 10.9
    }
  };

  function getProfile(profileId) {
    return PROFILES.dartboard;
  }

  function profileIds() {
    return ['dartboard'];
  }

  function buildScale(profile) {
    const p = PROFILES.dartboard;
    const coordRadiusMm = p.coordRadiusMm;
    const dsgPerMm = DSG_COORD_RANGE / coordRadiusMm;
    return {
      profileId: p.id,
      file: p.file,
      label: p.label,
      dsgPerSvgUnit: dsgPerMm,
      coordRadiusMm: coordRadiusMm,
      svgSize: 200,
      centerX: 100,
      centerY: 100,
      targetDiameterMm: p.targetDiameterMm,
      ring8RadiusMm: p.ring8RadiusMm,
      shotDiameterMm: p.shotDiameterMm,
      shotRadiusSvg: p.shotDiameterMm / 2,
      decMax: p.decMax,
      teilerBandDsg: p.teilerBandDsg,
      last10Max: p.last10Max
    };
  }

  window.SRTargetRegistry = {
    profiles: PROFILES,
    defaultProfileId: 'dartboard',
    ownerPluginId: 'darts',
    getProfile: getProfile,
    profileIds: profileIds,
    buildScale: buildScale,
    resolveProfileId: function () { return 'dartboard'; }
  };
})();
