# The Codebase Doctrine: Architecture, Principles, and Style

## 1. The Core Philosophy

### 1.1. Reality Over Abstraction
We reject the object-oriented lie that data and execution are the same thing. They are physically separate in hardware (Execute-only memory vs. Read/Write memory), and they must remain separate in our mental models. 
* **Data is inert.** It is the material.
* **Code is the tool.** It is the transformation.
* **We do not build "Managers" or "Handlers."** We build transformations that take data `A` and produce data `B`.

### 1.2. The Mountain Strategy
We do not build applications; we build capabilities.
* **Build the Mountain:** Your goal is to build a massive foundation of finished, reusable primitives (rendering, networking, serialization, math).
* **The Small House:** The actual application should be a "small house" sitting on top of this mountain. It should be thin glue code connecting these robust capabilities.
* **Implication:** If you are writing complex logic inside `main.go` or a specific app controller, you are failing. Move that logic into a generic, reusable library module.

### 1.3. Finish Your Software
We reject "Continuous Beta."
* **Definition of Done:** A module is done when you never have to touch it again. It becomes a trusted black box.
* **Technical Debt is a Myth:** There is only "Unfinished Work." If code is bad, rewrite it *now*. It will never be cheaper to fix than it is today.
* **Stable APIs:** Once a module is shared, its API is written in stone. If you need to change it, you create `ModuleV2` and keep `ModuleV1` alive. Breaking an API is a failure of foresight.

---

## 2. Data & Memory Architecture

### 2.1. The Strict Separation (No Methods)
**Rule:** You shall not use Go method receivers `func (s *Struct) Name()` for core logic.
**Exception:** Only when satisfying a standard library interface (e.g., `io.Reader`, `fmt.Stringer`).

**Reasoning:** Method receivers imply ownership and hidden state. They encourage "this-based" thinking where context is implicit. We want **Explicit Context**.
* **Bad:** `player.Update(dt)` (Hides what `Update` touches).
* **Good:** `PlayerUpdate(&player, dt, &worldState)` (Shows exactly what data is required and mutated).

### 2.2. Memory Layout & Padding
Memory is slow; Math is fast. A cache miss costs ~200 cycles. A multiplication costs 1.
* **Struct Layout:** Organize struct fields from largest (64-bit) to smallest (bool/byte) to minimize padding.
* **Padding Awareness:** Be aware that a `bool` followed by an `int64` wastes 7 bytes.
* **Contiguity:** Linked lists and pointer-heavy trees are "cache-miss machines." Prefer flat slices (arrays) and indices.

### 2.3. Initialization & "Zero Value" Danger
Go's default "zero value" is a trap. It leads to code that "sort of works" but is semantically wrong.
* **Debug Initialization:** In debug builds, initialize memory to garbage (e.g., `0xCD` or specific high-value integers) to force crashes if data is used before proper setup.
* **Partial Initialization:** If a struct requires 5 fields to be valid, and you only set 3 (relying on zero for the rest), you have created a bug that will only manifest rarely.

### 2.4. Handles vs. Pointers
For long-lived entities, prefer **Handles** (Indices) over Pointers.
* **Pointers die:** A pointer is invalid if the array reallocates or the program restarts.
* **Indices live:** An index (e.g., `uint32`) is easy to serialize, debug, and validate.
* **Opaque Handles:** When exposing a system to the outside, give them a `Handle` (opaque integer or struct), not a raw pointer. This allows you to reorganize memory internally without breaking the user's code.

---

## 3. Modularity & API Design

### 3.1. The Single-Person Mandate
A module must be small enough to be written, understood, and maintained by **one person**.
* If a module requires a meeting to understand, it is too big. Break it down.
* We synchronize via **APIs**, not meetings.

### 3.2. Black Boxes & Protocol Design
* **Internal vs. External:** A module consists of a public API and a private implementation. The user of the module cares *what* it does (capabilities), not *how* it does it (SQL vs. Flatfile).
* **The Glue Pattern:** When migrating or integrating systems, write small glue layers that translate Protocol A to Protocol B. Do not pollute the core logic with integration details.

### 3.3. The Stride Pattern
Do not force users to pack data exactly how you want it. Use **Strides**.
* Instead of `func Process(data []Vec3)`, use `func Process(ptr *float32, count int, stride int)`.
* This allows the user to store their data in `struct { Pos Vec3; Vel Vec3 }` (Interleaved) or `struct { Pos []Vec3 }` (Contiguous) and use your function without copying memory.

### 3.4. Plugin Architecture
For complex systems (e.g., editors, engines), use a **Core + Plugin** model.
* **The Core:** Manages the lifecycle, memory, and message passing. It knows nothing about specific logic (e.g., "Video Editing").
* **The Plugin:** Contains the logic. It registers itself with the core.
* This forces decoupling. If the Core needs to know about a specific plugin to compile, you have failed.

### 3.5. Library Organization & File Structure

**The Flat Earth Policy:** Do not create deep directory trees.
* **Directories Hide Code:** A deep folder structure (`src/network/tcp/client/async`) forces the developer to explore a tree to find code. A flat structure (`src/network`) with well-named files (`tcp_client_async.go`) puts everything in plain sight.
* **Package Density:** Prefer fewer, larger packages (libraries) over hundreds of "micro-packages." A package is a compilation unit and a namespace. Fragmenting it creates import cycles and API friction.

**The "Internal" Wall:**
* **Mandatory Usage:** The `internal/` folder is not a suggestion; it is the primary mechanism for API stability.
* **The Rule:** Put *everything* in `internal/` by default. Only move code to the root of the package when it is ready to be a stable, public API contract. The real API contract can be a single `api.go` file or multiple properly named files depending on library scope.
* **Enforcement:** This physically prevents other parts of the codebase from importing unfinished or private logic. It is the Go equivalent of static functions in C, but enforced by the compiler.

**File Granularity:**
* **Anti-fragmentation:** Do not create a file for every struct. This is Java madness.
* **Subsystem Grouping:** A file should contain a complete subsystem or behavior (e.g., `collision_detection.go` containing the structs, the math, and the logic).
* **File Count:** A package should contain as many files as necessary to be organized, but they must be flat. If a package reaches 50+ files, verify your naming convention (Section 4.1). Use prefixes to group files visually in the file explorer (`render_mesh.go`, `render_shader.go`, `render_texture.go`).
* **Line Count:** Do not fear large files (1000-2000 lines) if the code is linear and related. Fear files that import 20 other files to do one thing.

---

## 4. Coding Style & Conventions

### 4.1. "Wide Code" (Naming)
We do not fear typing. We fear ambiguity.
* **Descriptive Names:** `Create` is bad. `TextureCreateFromMemory` is good.
* **Prefixing:** Use C-style namespaces. `[System][Subsystem][Action]`.
    * `NetTCPConnect`
    * `UIWindowDraw`
    * `SimPhysicsStep`
* **No Overloading:** Go prevents function overloading, which is good. We want different names for different behaviors (e.g., `Vec3Add` vs `Vec3AddScalar`).

### 4.2. Formatting
* **Vertical Alignment:** Align assignments and declarations. It makes patterns (and deviations from patterns) instantly visible to the eye.
    ```go
    // Bad
    x := 10
    velocity := 20.5
    name := "Test"

    // Good
    x        := 10
    velocity := 20.5
    name     := "Test"
    ```

### 4.3. Control Flow
* **No Hidden Jumps:** No `defer` inside tight loops (performance). No `panic` for control flow.
* **Cyclomatic Complexity:** If you have deep nesting (`if` inside `for` inside `if`), extract the inner logic to a helper function.
* **Switch Cases:** Indent cases to align with the switch. Use explicit breaks or fallthroughs.

### 4.4. Comments & Documentation
**Principle:** Comments indicate a lack of expression in code and should be minimized.

* **Comments (`//`):** Use sparingly and only when absolutely critical.
    * **Critical Warnings:** Use for safety-critical information that cannot be expressed in code (e.g., "This function must be called before X or memory corruption will occur").
    * **Extremely Complex Algorithms:** Use only when the algorithm's logic is genuinely inexpressible through code structure (e.g., cryptographic primitives, complex mathematical proofs).
    * **No Step Separators:** Do not use comments to separate steps in sequential functions. Instead, refactor the function into an orchestrator that calls smaller, well-named functions. Each function should represent a single step.
        ```go
        // Bad
        func ProcessData(data []byte) {
            // Step 1: Validate input
            if len(data) == 0 {
                return
            }
            // Step 2: Parse header
            header := parseHeader(data)
            // Step 3: Transform data
            result := transform(header, data)
        }

        // Good
        func ProcessData(data []byte) {
            if !ValidateInput(data) {
                return
            }
            header := ParseHeader(data)
            result := TransformData(header, data)
        }
        ```

* **Documentation (`/* */`):** Required for every public function, type, and constant.
    * **Purpose:** Documentation describes *what* the function does, its parameters, return values, and any important behavioral notes.
    * **Format:** Use standard Go documentation style. The first sentence should be a summary.
    * **Scope:** All exported symbols (functions, types, constants, variables) must have documentation.
    * **Distinction:** Documentation (`/* */`) is for public APIs and describes the contract. Comments (`//`) are for internal implementation notes and should be rare.

### 4.5. Documentation Style

Documentation serves as the contract between the library and its users. It must be comprehensive, accurate, and structured for easy scanning.

#### 4.5.1. Package Documentation (`doc.go`)

Every package must have a `doc.go` file with package-level documentation.

* **Format:** Use `//` comments (not `/* */`) for package documentation.
* **Structure:**
    * First line: `// Package [name] provides [brief description].`
    * Follow with additional paragraphs separated by blank lines describing:
        * What the package does
        * How to use it
        * Key concepts or structures
        * Integration with other packages
* **Example:**
    ```go
    // Package core provides shared utilities for Statarch, including the core analysis structure
    // used across all statistical computation packages.
    //
    // The StatArchAnalysis structure provides a unified interface for caching computed statistics
    // and managing temporary vectors (sorted copies, scratch space) across descriptive, hypothesis,
    // and other statistical packages.
    package core
    ```

#### 4.5.2. Function Documentation

All public functions must have comprehensive documentation using `/* */` block comments.

* **Structure:** Function documentation follows a consistent structure:
    1. **Summary Sentence:** First sentence describes what the function does.
    2. **Extended Description:** (Optional) Additional paragraphs explaining the algorithm, formula, or concept.
    3. **Use Cases:** Bullet list of practical applications.
    4. **Time Complexity:** Big-O notation with brief explanation.
    5. **Space Complexity:** Big-O notation with brief explanation.
    6. **Prerequisites:** Requirements that must be met before calling.
    7. **Edge Cases:** Special cases, error conditions, or return value behaviors.
    8. **Formula:** (If applicable) Mathematical formula or algorithm description.
    9. **Additional Notes:** Caching behavior, side effects, or other important details.

* **Example:**
    ```go
    /*
    StatArchDescriptiveVectorMeanF32 computes the arithmetic mean of a vector in float32 precision.

    Use cases:
    - Measuring central tendency of data
    - Quality control and process monitoring
    - Feature analysis in machine learning
    - Financial computing (average returns, prices)

    Time complexity: O(n) - single pass through vector
    Space complexity: O(1) - only accumulator variables used

    Prerequisites:
    - Vector must contain at least 1 element
    - Vector does not need to be sorted

    Edge cases:
    - Returns 0 if vector is empty (count is 0)
    - Works with any numeric type (int, float32, float64, etc.)

    The function uses the analysis structure to cache the computed mean value.
    If the mean has already been computed, it returns the cached value.
    */
    func StatArchDescriptiveVectorMeanF32[T foundation.Numeric](
        analysis *core.StatArchAnalysis[T],
    ) float32 {
        // ...
    }
    ```

* **For Complex Functions:** Include algorithm descriptions, formulas, and mathematical context:
    ```go
    /*
    StatArchHypothesisVectorJarqueBeraF32 computes the Jarque-Bera test statistic in float32 precision.

    The Jarque-Bera test is a goodness-of-fit test that determines if sample data has
    skewness and kurtosis matching a normal distribution. The test statistic follows
    a chi-squared distribution with 2 degrees of freedom.

    Use cases:
    - Testing normality assumption (required for many statistical tests)
    - Quality control (checking if process data is normally distributed)
    - Model validation (checking residual normality)
    - Statistical inference (pre-test for parametric tests)

    Time complexity: O(1) - uses cached skewness and kurtosis from analysis
    Space complexity: O(1) - only local variables used

    Prerequisites:
    - analysis must be a valid StatArchAnalysis structure from statarch/core
    - Vector must contain at least 3 elements (population) or 4 elements (sample) for skewness/kurtosis
    - If sample=true, vector represents a sample from a larger population
    - If sample=false, vector represents the entire population

    Edge cases:
    - Panics if vector has insufficient elements (inherited from skewness/kurtosis requirements)
    - Test statistic is chi-squared distributed with 2 degrees of freedom
    - Large values indicate deviation from normality
    - Critical values: 5.99 (α=0.05), 9.21 (α=0.01)

    Formula: JB = (n/6) * (skewness² + (kurtosis²/4))
    */
    ```

#### 4.5.3. README Documentation

Libraries should include comprehensive README.md files that serve as the primary user-facing documentation.

* **Required Sections:**
    * **Overview:** Brief description of what the library does.
    * **Design Philosophy:** Core principles and architectural decisions.
    * **Performance Characteristics:** Zero allocations, cache efficiency, complexity guarantees.
    * **Integration:** Dependency relationships and how the library fits into the stack.
    * **Packages:** Documentation for each subpackage with:
        * Purpose and scope
        * Key structures and functions
        * Usage examples with code blocks
    * **Use Cases:** Practical applications of the library.
    * **Safety Guidelines:** Input validation, memory lifetime, error handling behavior.
    * **Implementation Notes:** (Optional) Details about specific algorithms or positive characteristics.
    * **Accuracy and Limitations:** (If applicable) Known limitations, numerical stability considerations, and recommendations.

* **Code Examples:** All examples must be complete, runnable code blocks showing:
    * Required imports
    * Setup (allocators, data structures)
    * Function calls with realistic parameters
    * Inline comments explaining key points

* **Formatting:**
    * Use clear section headers (`##`, `###`)
    * Use bullet lists for function catalogs
    * Use code blocks with language tags for all code examples
    * Use bold for emphasis on important warnings or notes
    * Use structured text diagrams for dependency relationships

* **Example Structure:**
    ```markdown
    # LibraryName

    Brief tagline describing the library.

    ## Overview

    Detailed description of what the library provides and its purpose.

    ## Design Philosophy

    - **Principle 1**: Explanation
    - **Principle 2**: Explanation

    ## Performance Characteristics

    - **Zero Allocations**: Description
    - **Cache Efficiency**: Description

    ## Integration

    Dependency diagram and relationships.

    ## Packages

    ### `package/subpackage`

    Description of the subpackage.

    **Functions:**
    - `FunctionName` - Description
    - `FunctionName2` - Description

    **Example:**

    ```go
    // Complete, runnable example
    ```

    ## Use Cases

    - **Use Case 1**: Description
    - **Use Case 2**: Description

    ## Safety Guidelines

    ⚠️ **Important:**

    1. **Guideline 1**: Description
    2. **Guideline 2**: Description
    ```

#### 4.5.4. Documentation Principles

* **Completeness:** Documentation must answer "what," "when," "why," and "how" for every public API.
* **Accuracy:** Documentation must match implementation. Outdated documentation is worse than no documentation.
* **Clarity:** Use precise language. Avoid ambiguity. Prefer concrete examples over abstract descriptions.
* **Consistency:** Follow the established structure across all functions in a package.
* **Practicality:** Include use cases and examples that show real-world application.
* **Transparency:** Document limitations, edge cases, and error conditions explicitly. Do not hide complexity.
* **Maintenance:** Update documentation when APIs change. Documentation is part of the code review process.

---

## 5. Debugging & Reliability

### 5.1. The "CSI" Mindset
You are both the murderer and the investigator.
* **Fail Fast:** We want runtime violations to crash the application immediately in Debug mode.
* **Validation Functions:** Implement `SystemValidate(state)` functions that check array bounds, enum validity, and pointer health. Call these aggressively in Debug builds.

### 5.2. Debugging Techniques
* **The No-Op Breakpoint:** Do not rely on IDE breakpoint persistence. Write them in code:
    ```go
    if debugCondition {
        _ = 0 // BREAKPOINT HERE
    }
    ```
* **Deterministic Counters:** Add static counters to functions. "Crash on the 435th call."
* **Magic Numbers:** When serializing data (files/network), insert "Magic Numbers" (e.g., `0x12345678`) between logical blocks. If the reader doesn't find the magic number, you know exactly where the stream desynchronized.

### 5.3. Visual Debugging
Don't just stare at code. Write tools.
* Visualizers for your data structures.
* Recorders for your inputs (so bugs can be replayed deterministically).
* Black box loggers that record the *inputs* and *outputs* of your modules.

---

## 6. Security & Performance

### 6.1. Performance is Ethical
Slow software wastes energy. It is a bug.
* **Minimize Dependencies:** Every external library is a risk (security and stability). Own your stack.
* **Zero Allocations:** In the hot loop (per frame/per request), there should be **zero** memory allocations. Allocate upfront, then reuse.

### 6.2. Security Orthodoxy
We reject "Security Theater" (e.g., annoying the user with constant prompts).
* **Real Security:** Comes from simplicity. A complex system cannot be secured. A simple system with a small surface area can be.
* **Buffer Overruns:** These are not the only bug. Logic bugs (ABA problems, race conditions) are harder to find and more dangerous. Focus on architecture correctness, not just boundary checks.

---

## 7. Implementation Directives (Go Specifics)

1.  **Slices:** Use them as "Views" into memory. Be careful of holding references to large underlying arrays via small slices.
2.  **Interfaces:** Use them *only* for the API boundary to allow multiple implementations (e.g., `BackendOpenGL` vs `BackendVulkan`). Do not use them for internal polymorphism inside a hot loop.
3.  **Concurrency:**
    * Prefer **Authoritative Core + Subscribers**.
    * One central routine owns the state. Others subscribe to updates.
    * Avoid shared memory with mutexes if possible. Use message passing (Channels) for coordination, but be wary of channel blocking/deadlocks. 
    * **Debug Toggle:** You must be able to turn off concurrency to debug logic errors.

## 8. Summary Checklist
Before committing code, ask:
1.  **Is it finished?** (Or am I planning to fix it "later"?)
2.  **Is it explicit?** (Did I use a method receiver? Did I hide state?)
3.  **Is it robust?** (Did I handle the zero case? Did I add validators?)
4.  **Is it independent?** (Does this module require the rest of the app to compile?)

**Build the Mountain.**