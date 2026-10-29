-- Upwork-style freelancer marketplace: Projects table
-- Identity in this schema is candidate_profiles / members (no users table).
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID REFERENCES jobs(id) ON DELETE CASCADE,
    project_type VARCHAR(50) NOT NULL CHECK (project_type IN ('fixed_price', 'hourly', 'retainer', 'milestone')),
    budget_min DECIMAL(10,2),
    budget_max DECIMAL(10,2),
    currency VARCHAR(3) DEFAULT 'USD',
    duration_estimate VARCHAR(50), -- '1-2 weeks', '1 month', etc.
    skills_required TEXT[] DEFAULT '{}',
    deliverables TEXT[] DEFAULT '{}',
    milestones JSONB, -- [{name, amount, due_date, completed_at}]
    bidding_end_at TIMESTAMPTZ,
    max_bids INTEGER DEFAULT 30,
    escrow_required BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_projects_job_id ON projects(job_id);
CREATE INDEX IF NOT EXISTS idx_projects_bidding_end ON projects(bidding_end_at) WHERE bidding_end_at IS NOT NULL;

-- Bids table
CREATE TABLE IF NOT EXISTS bids (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    freelancer_id UUID NOT NULL REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    bid_amount DECIMAL(10,2),
    hourly_rate DECIMAL(10,2),
    estimated_hours INTEGER,
    proposal TEXT NOT NULL,
    delivery_timeline VARCHAR(100),
    attachments JSONB DEFAULT '[]',
    status VARCHAR(50) NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'shortlisted', 'rejected', 'accepted', 'withdrawn')),
    is_shortlisted BOOLEAN DEFAULT false,
    client_rating INTEGER CHECK (client_rating BETWEEN 1 AND 5),
    client_feedback TEXT,
    freelancer_rating INTEGER CHECK (freelancer_rating BETWEEN 1 AND 5),
    freelancer_feedback TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, freelancer_id)
);

CREATE INDEX IF NOT EXISTS idx_bids_project_id ON bids(project_id);
CREATE INDEX IF NOT EXISTS idx_bids_freelancer_id ON bids(freelancer_id);
CREATE INDEX IF NOT EXISTS idx_bids_status ON bids(status);
CREATE INDEX IF NOT EXISTS idx_bids_shortlisted ON bids(project_id) WHERE is_shortlisted = true;

-- Freelancer profiles (extends candidate_profiles)
CREATE TABLE IF NOT EXISTS freelancer_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    candidate_profile_id UUID NOT NULL UNIQUE REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    hourly_rate_min DECIMAL(10,2),
    hourly_rate_max DECIMAL(10,2),
    availability VARCHAR(50) CHECK (availability IN ('full_time', 'part_time', 'not_available')),
    portfolio_urls TEXT[] DEFAULT '{}',
    years_experience INTEGER,
    success_score DECIMAL(3,2) DEFAULT 0.0,
    completed_projects INTEGER DEFAULT 0,
    client_satisfaction DECIMAL(3,2) DEFAULT 0.0,
    on_time_delivery DECIMAL(3,2) DEFAULT 0.0,
    skills_certifications JSONB DEFAULT '{}',
    verified_identity BOOLEAN DEFAULT false,
    payment_methods JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_freelancer_profiles_candidate_id ON freelancer_profiles(candidate_profile_id);

-- Project categories for better filtering
CREATE TABLE IF NOT EXISTS project_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    icon VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO project_categories (name, slug, description, icon) VALUES
    ('Web Development', 'web-development', 'Frontend, backend, full-stack development projects', 'code'),
    ('Mobile Development', 'mobile-development', 'iOS, Android, cross-platform app development', 'smartphone'),
    ('Design & Creative', 'design-creative', 'UI/UX design, graphic design, video editing', 'palette'),
    ('Writing & Content', 'writing-content', 'Content writing, copywriting, blogging, editing', 'edit'),
    ('Marketing & Sales', 'marketing-sales', 'Digital marketing, SEO, social media, sales', 'trending-up'),
    ('Administrative Support', 'admin-support', 'Virtual assistance, data entry, customer service', 'file-text'),
    ('Consulting & Strategy', 'consulting-strategy', 'Business consulting, strategy planning', 'briefcase'),
    ('AI & Machine Learning', 'ai-ml', 'AI development, ML models, data science', 'brain')
ON CONFLICT (slug) DO NOTHING;

-- Project to categories mapping
CREATE TABLE IF NOT EXISTS project_categories_mapping (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES project_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, category_id)
);

CREATE INDEX IF NOT EXISTS idx_project_categories_project ON project_categories_mapping(project_id);
CREATE INDEX IF NOT EXISTS idx_project_categories_category ON project_categories_mapping(category_id);
