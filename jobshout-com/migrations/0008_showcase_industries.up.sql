-- AI Showcase: browse by industry. A two-level taxonomy (industry, then an
-- optional vertical) lives in showcase_industries — the one source the API
-- serves and validates against. An entry stores its industries and verticals
-- together in showcase_apps.industries (a vertical always with its parent), so
-- an industry filter is one indexed array test.

CREATE TABLE IF NOT EXISTS showcase_industries (
    slug         TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    parent_slug  TEXT REFERENCES showcase_industries(slug) ON DELETE CASCADE,
    description  TEXT NOT NULL DEFAULT '',
    position     INT  NOT NULL DEFAULT 0
);

-- Industries first: verticals reference them.
INSERT INTO showcase_industries (slug, name, parent_slug, description, position) VALUES
    ('healthcare',            'Healthcare & life sciences',   NULL, 'Hospitals, the NHS, GPs, pharma, medical devices and care.', 1),
    ('financial-services',    'Financial services',           NULL, 'Banks, insurers, wealth managers, payments and lending.', 2),
    ('accountancy',           'Accountancy & tax',            NULL, 'Bookkeeping, audit, tax and payroll.', 3),
    ('legal',                 'Legal',                        NULL, 'Law firms, in-house legal teams, compliance and conveyancing.', 4),
    ('public-sector',         'Public sector & government',   NULL, 'Central and local government, defence, justice and policing.', 5),
    ('education',             'Education',                    NULL, 'Schools, universities and workplace training.', 6),
    ('retail',                'Retail & e-commerce',          NULL, 'Shops, online stores and consumer brands.', 7),
    ('manufacturing',         'Manufacturing & engineering',  NULL, 'Factories, industrial engineering and supply chains.', 8),
    ('construction-property', 'Construction & property',      NULL, 'Building, real estate and facilities management.', 9),
    ('energy-utilities',      'Energy & utilities',           NULL, 'Power, water, oil and gas, and renewables.', 10),
    ('transport-logistics',   'Transport & logistics',        NULL, 'Freight, fleets, warehousing and travel networks.', 11),
    ('media-marketing',       'Media, marketing & creative',  NULL, 'Publishing, broadcasting, advertising and agencies.', 12),
    ('telecoms-it',           'Telecoms & IT services',       NULL, 'Networks, managed services and IT consultancies.', 13),
    ('hospitality-travel',    'Hospitality, travel & leisure', NULL, 'Hotels, restaurants, travel and leisure.', 14),
    ('agriculture-food',      'Agriculture & food',           NULL, 'Farming, food production and supply.', 15),
    ('recruitment-hr',        'Recruitment & HR',             NULL, 'Recruiters, staffing agencies and HR teams.', 16),
    ('charities',             'Charities & non-profit',       NULL, 'Charities, foundations and social enterprises.', 17),
    ('cross-industry',        'Works across industries',      NULL, 'General-purpose tools any sector can use.', 99)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO showcase_industries (slug, name, parent_slug, description, position) VALUES
    ('hospitals-clinics',  'Hospitals & clinics',            'healthcare', 'NHS trusts, private hospitals and clinics.', 1),
    ('primary-care',       'Primary care & GP',              'healthcare', 'GP practices and primary care networks.', 2),
    ('pharma-biotech',     'Pharma & biotech',               'healthcare', '', 3),
    ('medical-devices',    'Medical devices',                'healthcare', '', 4),
    ('social-care',        'Social & home care',             'healthcare', '', 5),
    ('mental-health',      'Mental health',                  'healthcare', '', 6),
    ('banking',            'Banking',                        'financial-services', '', 1),
    ('insurance',          'Insurance',                      'financial-services', '', 2),
    ('wealth-management',  'Wealth & asset management',      'financial-services', '', 3),
    ('payments-fintech',   'Payments & fintech',             'financial-services', '', 4),
    ('lending-credit',     'Lending & credit',               'financial-services', '', 5),
    ('bookkeeping',        'Bookkeeping',                    'accountancy', '', 1),
    ('audit',              'Audit & assurance',              'accountancy', '', 2),
    ('tax',                'Tax',                            'accountancy', '', 3),
    ('payroll',            'Payroll',                        'accountancy', '', 4),
    ('law-firms',          'Law firms',                      'legal', '', 1),
    ('in-house-legal',     'In-house legal',                 'legal', '', 2),
    ('compliance',         'Compliance & regulatory',        'legal', '', 3),
    ('conveyancing',       'Conveyancing & property law',    'legal', '', 4),
    ('central-government', 'Central government',             'public-sector', '', 1),
    ('local-government',   'Local government',               'public-sector', '', 2),
    ('defence',            'Defence & security',             'public-sector', '', 3),
    ('justice',            'Justice & policing',             'public-sector', '', 4),
    ('schools',            'Schools',                        'education', '', 1),
    ('higher-education',   'Higher education',               'education', '', 2),
    ('training',           'Training & L&D',                 'education', '', 3),
    ('retail-stores',      'Retail stores',                  'retail', '', 1),
    ('ecommerce',          'E-commerce',                     'retail', '', 2),
    ('consumer-goods',     'Consumer goods',                 'retail', '', 3),
    ('construction',       'Construction',                   'construction-property', '', 1),
    ('real-estate',        'Real estate',                    'construction-property', '', 2),
    ('facilities',         'Facilities management',          'construction-property', '', 3)
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS industries TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS showcase_apps_industries_idx
    ON showcase_apps USING GIN (industries);
