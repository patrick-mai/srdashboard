/**
 * Hardcoded target geometry profiles for Classic Range.
 * Scoring values come from OpticScore; profiles control face SVG and zoom/scale only.
 */
(function () {
  const DSG_COORD_RANGE = 9000;

  const PROFILES = {
    air_rifle_10m: {
      id: 'air_rifle_10m',
      label: '10 m Luftgewehr (ISSF)',
      file: '10_m_Air_Rifle_target.svg',
      targetDiameterMm: 45.5,
      tenRingDiameterMm: 0.5,
      innerTenDiameterMm: 0.5,
      ringWidthMm: 2.5,
      oneRingSvg: 2.5,
      ring8RadiusMm: 5.25,
      shotDiameterMm: 4.5,
      decMax: 10.9,
      /** DISAG coord radius this profile maps to ±9000 (mm). Rifle: 200 mm frame (log-verified). */
      coordRadiusMm: 100,
      teilerBandDsg: 25,
      last10Max: 10.9
    },
    air_pistol_10m: {
      id: 'air_pistol_10m',
      label: '10 m Luftpistole (ISSF)',
      file: '10_m_Air_Pistol_target.svg',
      targetDiameterMm: 155.5,
      tenRingDiameterMm: 11.5,
      innerTenDiameterMm: 5.0,
      ringWidthMm: 8.0,
      oneRingSvg: 8.0,
      ring8RadiusMm: 21.75,
      shotDiameterMm: 4.5,
      decMax: 10.9,
      /**
       * TODO(refine): LP X/Y scale vs rifle not log-verified yet. coordRadiusMm is provisional;
       * adjust when a real OpticScore pistol shot (DecValue + X/Y/Distance) is available.
       */
      coordRadiusMm: 130,
      coordRadiusNeedsRefinement: true,
      teilerBandDsg: 72,
      last10Max: 10.9
    },
    smallbore_50m_prone: {
      id: 'smallbore_50m_prone',
      label: '50 m KK liegend',
      file: '50_m_Smallbore_target.svg',
      targetDiameterMm: 154.4,
      tenRingDiameterMm: 10.4,
      oneRingSvg: 8.0,
      ring8RadiusMm: 52.0,
      shotDiameterMm: 5.6,
      last10Max: 10.9
    },
    smallbore_50m_3p: {
      id: 'smallbore_50m_3p',
      label: '50 m KK 3-Stellung',
      file: '50_m_Smallbore_target.svg',
      targetDiameterMm: 154.4,
      tenRingDiameterMm: 10.4,
      oneRingSvg: 8.0,
      ring8RadiusMm: 52.0,
      shotDiameterMm: 5.6,
      last10Max: 10.9
    }
  };

  const DEFAULT_PROFILE_ID = 'air_rifle_10m';

  function getProfile(profileId) {
    return PROFILES[profileId] || PROFILES[DEFAULT_PROFILE_ID];
  }

  function profileIds() {
    return Object.keys(PROFILES);
  }

  function buildScale(profile) {
    const p = profile || PROFILES[DEFAULT_PROFILE_ID];
    const RANGE_DIAMETER_MM = 200;
    const coordRadiusMm = p.coordRadiusMm != null ? p.coordRadiusMm : 100;
    const dsgPerMm = DSG_COORD_RANGE / coordRadiusMm;
    const DEFAULT_SVG_CENTER = 100;
    const DEFAULT_SVG_VIEW_SIZE = 200;
    const shotRadiusSvg = (p.shotDiameterMm / 2) / (RANGE_DIAMETER_MM / 2) * DEFAULT_SVG_CENTER;
    return {
      profileId: p.id,
      file: p.file,
      label: p.label,
      dsgPerSvgUnit: dsgPerMm,
      coordRadiusMm: coordRadiusMm,
      svgSize: DEFAULT_SVG_VIEW_SIZE,
      centerX: DEFAULT_SVG_CENTER,
      centerY: DEFAULT_SVG_CENTER,
      targetDiameterMm: p.targetDiameterMm,
      tenRingDiameterMm: p.tenRingDiameterMm,
      innerTenDiameterMm: p.innerTenDiameterMm,
      ringWidthMm: p.ringWidthMm,
      oneRingSvg: p.oneRingSvg,
      ring8RadiusMm: p.ring8RadiusMm,
      shotDiameterMm: p.shotDiameterMm,
      shotRadiusSvg: shotRadiusSvg,
      decMax: p.decMax != null ? p.decMax : 10.9,
      teilerBandDsg: p.teilerBandDsg,
      last10Max: p.last10Max != null ? p.last10Max : (p.decMax != null ? p.decMax : 10.9)
    };
  }

  window.SRTargetRegistry = {
    profiles: PROFILES,
    defaultProfileId: DEFAULT_PROFILE_ID,
    getProfile: getProfile,
    profileIds: profileIds,
    buildScale: buildScale
  };
})();
