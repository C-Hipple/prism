package service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// getTestFilePaths attempts to find the corresponding test file for a given source file
// It returns a slice of potential test file paths to try, ordered by likelihood
func (s *Service) getTestFilePaths(filePath string) []string {
	var testPaths []string

	// Handle Go files
	if strings.HasSuffix(filePath, ".go") && !strings.HasSuffix(filePath, "_test.go") {
		base := strings.TrimSuffix(filePath, ".go")
		testPaths = append(testPaths, base+"_test.go") // Same directory pattern

		// Handle different directory structures
		if strings.Contains(filePath, "/") {
			parts := strings.Split(filePath, "/")
			fileName := parts[len(parts)-1]
			dirPath := strings.Join(parts[:len(parts)-1], "/")
			fileBase := strings.TrimSuffix(fileName, ".go")

			// Common Go test directory patterns
			testPaths = append(testPaths,
				"test/"+dirPath+"/"+fileBase+"_test.go",  // test/pkg/service/auth_test.go
				dirPath+"/test/"+fileBase+"_test.go",     // pkg/service/test/auth_test.go
				"tests/"+dirPath+"/"+fileBase+"_test.go", // tests/pkg/service/auth_test.go
			)
		}
	}

	// Handle Python files
	if strings.HasSuffix(filePath, ".py") && !strings.Contains(filePath, "test") {
		if strings.Contains(filePath, "/") {
			parts := strings.Split(filePath, "/")
			fileName := parts[len(parts)-1]
			dirPath := strings.Join(parts[:len(parts)-1], "/")
			fileBase := strings.TrimSuffix(fileName, ".py")

			// Python test patterns
			testPaths = append(testPaths,
				// Same directory patterns
				dirPath+"/"+fileBase+"_test.py",
				dirPath+"/test_"+fileName,

				// Common Python test directory patterns
				"tests/"+dirPath+"/test_"+fileName, // tests/app/models/test_user.py
				"test/"+dirPath+"/test_"+fileName,  // test/app/models/test_user.py
				dirPath+"/tests/test_"+fileName,    // app/models/tests/test_user.py
				dirPath+"/test/test_"+fileName,     // app/models/test/test_user.py
				"tests/"+filePath,                  // tests/app/models/user.py (same structure)
				"test/"+filePath,                   // test/app/models/user.py
			)
		} else {
			fileBase := strings.TrimSuffix(filePath, ".py")
			testPaths = append(testPaths,
				fileBase+"_test.py",
				"test_"+filePath,
				"tests/test_"+filePath,
				"test/test_"+filePath,
			)
		}
	}

	// Handle JavaScript/TypeScript files
	if strings.HasSuffix(filePath, ".js") || strings.HasSuffix(filePath, ".ts") ||
		strings.HasSuffix(filePath, ".jsx") || strings.HasSuffix(filePath, ".tsx") {

		ext := ""
		base := ""
		if strings.HasSuffix(filePath, ".jsx") {
			ext = ".jsx"
			base = strings.TrimSuffix(filePath, ".jsx")
		} else if strings.HasSuffix(filePath, ".tsx") {
			ext = ".tsx"
			base = strings.TrimSuffix(filePath, ".tsx")
		} else if strings.HasSuffix(filePath, ".ts") {
			ext = ".ts"
			base = strings.TrimSuffix(filePath, ".ts")
		} else {
			ext = ".js"
			base = strings.TrimSuffix(filePath, ".js")
		}

		// Same directory patterns
		testPaths = append(testPaths,
			base+".test"+ext,
			base+".spec"+ext,
		)

		// Handle different directory structures
		if strings.Contains(filePath, "/") {
			parts := strings.Split(filePath, "/")
			fileName := parts[len(parts)-1]
			dirPath := strings.Join(parts[:len(parts)-1], "/")
			fileBase := strings.TrimSuffix(fileName, ext)

			// Common JS/TS test directory patterns
			testPaths = append(testPaths,
				"__tests__/"+dirPath+"/"+fileBase+".test"+ext, // __tests__/src/components/Button.test.tsx
				"tests/"+dirPath+"/"+fileBase+".test"+ext,     // tests/src/components/Button.test.tsx
				"test/"+dirPath+"/"+fileBase+".test"+ext,      // test/src/components/Button.test.tsx
				dirPath+"/__tests__/"+fileBase+".test"+ext,    // src/components/__tests__/Button.test.tsx
				dirPath+"/tests/"+fileBase+".test"+ext,        // src/components/tests/Button.test.tsx
				dirPath+"/test/"+fileBase+".test"+ext,         // src/components/test/Button.test.tsx

				// Spec variants
				"__tests__/"+dirPath+"/"+fileBase+".spec"+ext,
				"tests/"+dirPath+"/"+fileBase+".spec"+ext,
				dirPath+"/__tests__/"+fileBase+".spec"+ext,
			)
		}
	}

	return testPaths
}

// findTestFilesByStaticAnalysis uses static analysis to find test files that actually import/reference the source file
func (s *Service) findTestFilesByStaticAnalysis(token, owner, repoName, sourceFile, baseBranch string) []string {
	var testFiles []string

	// Handle Go files with AST parsing
	if strings.HasSuffix(sourceFile, ".go") && !strings.HasSuffix(sourceFile, "_test.go") {
		testFiles = append(testFiles, s.findGoTestFilesByImports(token, owner, repoName, sourceFile, baseBranch)...)
	}

	// Handle other languages with regex-based import analysis
	if strings.HasSuffix(sourceFile, ".py") ||
		strings.HasSuffix(sourceFile, ".js") ||
		strings.HasSuffix(sourceFile, ".ts") ||
		strings.HasSuffix(sourceFile, ".jsx") ||
		strings.HasSuffix(sourceFile, ".tsx") {
		// First, try direct import analysis
		directTestFiles := s.findTestFilesByImportAnalysis(token, owner, repoName, sourceFile, baseBranch)
		testFiles = append(testFiles, directTestFiles...)

		// Then, try broader package-level and semantic analysis
		packageTestFiles := s.findTestFilesByPackageAnalysis(token, owner, repoName, sourceFile, baseBranch)
		testFiles = append(testFiles, packageTestFiles...)

		// Finally, try semantic content analysis
		semanticTestFiles := s.findTestFilesBySemanticAnalysis(token, owner, repoName, sourceFile, baseBranch)
		testFiles = append(testFiles, semanticTestFiles...)
	}

	// Remove duplicates
	testFiles = s.removeDuplicateStrings(testFiles)

	return testFiles
}

// findGoTestFilesByImports analyzes Go test files using AST to find actual imports and references
func (s *Service) findGoTestFilesByImports(token, owner, repoName, sourceFile, baseBranch string) []string {
	var testFiles []string

	// Get potential test file patterns first
	potentialPaths := s.getTestFilePaths(sourceFile)

	// For Go, we can analyze the package structure
	sourcePackage := filepath.Dir(sourceFile)
	if sourcePackage == "." {
		sourcePackage = ""
	}

	for _, testPath := range potentialPaths {
		if !strings.HasSuffix(testPath, "_test.go") {
			continue
		}

		// Fetch the test file content
		content, err := s.githubClient.GetFileContent(token, owner, repoName, testPath, baseBranch)
		if err != nil {
			continue // File doesn't exist
		}

		// Parse the Go file to check for actual references
		if s.goFileReferencesSource(content, sourceFile, sourcePackage) {
			testFiles = append(testFiles, testPath)
		}
	}

	return testFiles
}

// goFileReferencesSource uses Go AST parsing to check if a test file references the source file
func (s *Service) goFileReferencesSource(testContent, sourceFile, sourcePackage string) bool {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", testContent, parser.ParseComments)
	if err != nil {
		// Fallback to simple string matching if parsing fails
		return s.simpleImportAnalysis(testContent, sourceFile)
	}

	sourceFileName := filepath.Base(sourceFile)
	sourceFileBase := strings.TrimSuffix(sourceFileName, ".go")

	// Check if the test file is in the same package (Go convention)
	if sourcePackage != "" {
		testPackage := node.Name.Name
		expectedPackage := filepath.Base(sourcePackage)
		if testPackage == expectedPackage {
			return true // Same package, likely tests the source file
		}
	}

	// Check imports
	for _, imp := range node.Imports {
		if imp.Path != nil {
			importPath := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(importPath, sourcePackage) {
				return true
			}
		}
	}

	// Check for function calls that might reference the source file
	var hasReference bool
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					// Look for package.Function() calls
					if strings.Contains(ident.Name, sourceFileBase) {
						hasReference = true
						return false
					}
				}
			}
		case *ast.Ident:
			// Look for direct references to types/functions from source file
			if strings.Contains(x.Name, sourceFileBase) {
				hasReference = true
				return false
			}
		}
		return true
	})

	return hasReference
}

// findTestFilesByImportAnalysis uses regex-based analysis for non-Go files
func (s *Service) findTestFilesByImportAnalysis(token, owner, repoName, sourceFile, baseBranch string) []string {
	var testFiles []string

	// Get potential test file patterns
	potentialPaths := s.getTestFilePaths(sourceFile)

	for _, testPath := range potentialPaths {
		// Fetch the test file content
		content, err := s.githubClient.GetFileContent(token, owner, repoName, testPath, baseBranch)
		if err != nil {
			continue // File doesn't exist
		}

		// Check if this test file actually imports/references the source file
		if s.simpleImportAnalysis(content, sourceFile) {
			testFiles = append(testFiles, testPath)
		}
	}

	return testFiles
}

// simpleImportAnalysis uses regex patterns to find import statements that reference the source file
func (s *Service) simpleImportAnalysis(testContent, sourceFile string) bool {
	sourceFileName := filepath.Base(sourceFile)
	sourceFileBase := strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName))
	sourceDir := filepath.Dir(sourceFile)

	// Convert file path to Python module path (e.g., "tipping/views.py" -> "tipping.views")
	pythonModulePath := strings.ReplaceAll(sourceDir, "/", ".")
	if pythonModulePath != "" && sourceFileBase != "__init__" {
		pythonModulePath = pythonModulePath + "." + sourceFileBase
	} else if pythonModulePath == "" {
		pythonModulePath = sourceFileBase
	}

	patterns := s.buildImportPatterns(sourceFile, sourceFileBase, sourceDir, pythonModulePath)

	return matchAnyPattern(patterns, testContent)
}

// buildImportPatterns creates all import patterns for different languages
func (s *Service) buildImportPatterns(sourceFile, sourceFileBase, sourceDir, pythonModulePath string) []string {
	var patterns []string

	// Python patterns
	patterns = append(patterns, s.buildPythonPatterns(sourceFileBase, sourceDir, pythonModulePath)...)

	// JavaScript/TypeScript patterns
	if strings.HasSuffix(sourceFile, ".js") || strings.HasSuffix(sourceFile, ".ts") ||
		strings.HasSuffix(sourceFile, ".jsx") || strings.HasSuffix(sourceFile, ".tsx") {
		patterns = append(patterns, s.buildJSPatterns(sourceFileBase)...)
	}

	// Go patterns
	if strings.HasSuffix(sourceFile, ".go") {
		patterns = append(patterns, s.buildGoPatterns(sourceFileBase, sourceDir)...)
	}

	return patterns
}

// buildPythonPatterns creates Python import patterns
func (s *Service) buildPythonPatterns(sourceFileBase, sourceDir, pythonModulePath string) []string {
	packagePath := strings.ReplaceAll(sourceDir, "/", ".")

	return []string{
		// Direct imports
		`import\s+` + regexp.QuoteMeta(pythonModulePath) + `(\s|$|,|;)`,
		`import\s+` + regexp.QuoteMeta(sourceFileBase) + `(\s|$|,|;)`,
		`import\s+` + regexp.QuoteMeta(pythonModulePath) + `\s+as\s+\w+`,

		// From imports
		`from\s+` + regexp.QuoteMeta(packagePath) + `\s+import\s+.*` + regexp.QuoteMeta(sourceFileBase),
		`from\s+` + regexp.QuoteMeta(pythonModulePath) + `\s+import`,
		`from\s+` + regexp.QuoteMeta(pythonModulePath) + `\s+import\s+\*`,

		// Relative imports
		`from\s+\.+` + regexp.QuoteMeta(sourceFileBase) + `\s+import`,
		`from\s+\.+.*` + regexp.QuoteMeta(pythonModulePath) + `\s+import`,

		// Dynamic imports
		`importlib\.import_module\s*\(\s*['"]` + regexp.QuoteMeta(pythonModulePath) + `['"]`,
		`__import__\s*\(\s*['"]` + regexp.QuoteMeta(pythonModulePath) + `['"]`,

		// Usage patterns
		regexp.QuoteMeta(sourceFileBase) + `\.`,
		`(mock|patch)\s*\(\s*['"]` + regexp.QuoteMeta(pythonModulePath) + `\.`,
		`['"]` + regexp.QuoteMeta(pythonModulePath) + `['"]`,
		`#.*` + regexp.QuoteMeta(sourceFileBase),
	}
}

// buildJSPatterns creates JavaScript/TypeScript import patterns
func (s *Service) buildJSPatterns(sourceFileBase string) []string {
	return []string{
		// ES6 imports
		`import\s+\w+\s+from\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`import\s+\{[^}]*\}\s+from\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`import\s+\*\s+as\s+\w+\s+from\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`import\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// CommonJS
		`require\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`const\s+\{[^}]*\}\s*=\s*require\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// Dynamic imports
		`import\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`await\s+import\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// Re-exports
		`export\s+.*from\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// TypeScript types
		`import\s+type\s+.*from\s+['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// Framework specific
		`(jest|vi)\.mock\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`dynamic\s*\(\s*\(\s*\)\s*=>\s*import\s*\(\s*['"].*` + regexp.QuoteMeta(sourceFileBase) + `['"]`,

		// Usage patterns
		`</?` + regexp.QuoteMeta(sourceFileBase) + `[\s/>]`,
		regexp.QuoteMeta(sourceFileBase) + `\s*\(`,
		`['"]` + regexp.QuoteMeta(sourceFileBase) + `['"]`,
		`//.*` + regexp.QuoteMeta(sourceFileBase),
	}
}

// buildGoPatterns creates Go import patterns
func (s *Service) buildGoPatterns(sourceFileBase, sourceDir string) []string {
	return []string{
		`import\s+.*` + regexp.QuoteMeta(sourceDir),
		regexp.QuoteMeta(sourceFileBase) + `\.`,
		`func\s+Test.*` + regexp.QuoteMeta(cases.Title(language.English).String(sourceFileBase)),
	}
}

// findTestFilesByPackageAnalysis finds test files that test the entire package containing the source file
func (s *Service) findTestFilesByPackageAnalysis(token, owner, repoName, sourceFile, baseBranch string) []string {
	var testFiles []string

	sourceDir := filepath.Dir(sourceFile)
	sourceFileName := filepath.Base(sourceFile)
	sourceFileBase := strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName))

	// Get the package name (last part of the directory path)
	packageName := filepath.Base(sourceDir)
	if packageName == "." || packageName == "" {
		packageName = sourceFileBase
	}

	// Generate broader test file patterns that might test the entire package
	packageTestPatterns := s.generatePackageTestPatterns(sourceFile, packageName)

	for _, testPath := range packageTestPatterns {
		// Fetch the test file content
		content, err := s.githubClient.GetFileContent(token, owner, repoName, testPath, baseBranch)
		if err != nil {
			continue // File doesn't exist
		}

		// Check if this test file references the package or contains integration tests
		if s.packageLevelAnalysis(content, sourceFile, packageName) {
			testFiles = append(testFiles, testPath)
		}
	}

	return testFiles
}

// generatePackageTestPatterns creates test file patterns for package-level testing
func (s *Service) generatePackageTestPatterns(sourceFile, packageName string) []string {
	sourceDir := filepath.Dir(sourceFile)

	if strings.HasSuffix(sourceFile, ".py") {
		return s.generatePythonPackagePatterns(sourceDir, packageName)
	}

	if strings.HasSuffix(sourceFile, ".js") || strings.HasSuffix(sourceFile, ".ts") ||
		strings.HasSuffix(sourceFile, ".jsx") || strings.HasSuffix(sourceFile, ".tsx") {
		return s.generateJSPackagePatterns(sourceFile, sourceDir, packageName)
	}

	return []string{}
}

// generatePythonPackagePatterns creates Python package test patterns
func (s *Service) generatePythonPackagePatterns(sourceDir, packageName string) []string {
	return []string{
		// Same directory
		sourceDir + "/test_" + packageName + ".py",
		sourceDir + "/" + packageName + "_test.py",

		// Tests subdirectories
		sourceDir + "/tests/test_" + packageName + ".py",
		sourceDir + "/test/test_" + packageName + ".py",
		"tests/" + sourceDir + "/test_" + packageName + ".py",
		"test/" + sourceDir + "/test_" + packageName + ".py",

		// Integration tests
		sourceDir + "/tests/test_integration.py",
		"tests/" + sourceDir + "/test_integration.py",
		"tests/integration/test_" + packageName + ".py",

		// Django patterns
		sourceDir + "/tests/test_" + packageName + "_views.py",
		sourceDir + "/tests/test_" + packageName + "_models.py",

		// Functional tests
		"tests/functional/test_" + packageName + ".py",
		"tests/e2e/test_" + packageName + ".py",
	}
}

// generateJSPackagePatterns creates JavaScript/TypeScript package test patterns
func (s *Service) generateJSPackagePatterns(sourceFile, sourceDir, packageName string) []string {
	ext := ".test.js"
	if strings.Contains(sourceFile, ".ts") {
		ext = ".test.ts"
	}

	return []string{
		sourceDir + "/" + packageName + ext,
		sourceDir + "/__tests__/" + packageName + ext,
		"__tests__/" + sourceDir + "/" + packageName + ext,
		sourceDir + "/__tests__/integration" + ext,
		"__tests__/integration/" + packageName + ext,
		"tests/integration/" + packageName + ext,
	}
}

// packageLevelAnalysis checks if a test file contains package-level or integration tests
func (s *Service) packageLevelAnalysis(testContent, sourceFile, packageName string) bool {
	sourceDir := filepath.Dir(sourceFile)
	sourceFileBase := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	packagePath := strings.ReplaceAll(sourceDir, "/", ".")

	patterns := []string{
		// Package imports
		`from\s+` + regexp.QuoteMeta(packageName) + `\s+import`,
		`import\s+` + regexp.QuoteMeta(packageName),
		`from\s+` + regexp.QuoteMeta(packagePath) + `\s+import`,

		// Integration test indicators
		`(integration|functional|e2e).*test`,
		`test.*(integration|functional|e2e)`,

		// Test classes and functions
		`class.*Test.*` + regexp.QuoteMeta(cases.Title(language.English).String(packageName)),
		`def\s+test_` + regexp.QuoteMeta(packageName),
		`def\s+test.*` + regexp.QuoteMeta(sourceFileBase),

		// Django patterns
		`(client|self)\.get\s*\(\s*['"].*` + regexp.QuoteMeta(packageName),
		`reverse\s*\(\s*['"]` + regexp.QuoteMeta(packageName),
		`\.objects\.(create|get|filter)`,

		// API testing
		`(GET|POST|PUT|DELETE|PATCH).*` + regexp.QuoteMeta(packageName),
		`api.*` + regexp.QuoteMeta(packageName),
	}

	return matchAnyPattern(patterns, testContent)
}

// findTestFilesBySemanticAnalysis finds test files by analyzing content for function/class references
func (s *Service) findTestFilesBySemanticAnalysis(token, owner, repoName, sourceFile, baseBranch string) []string {
	var testFiles []string

	// First, extract function and class names from the source file
	sourceContent, err := s.githubClient.GetFileContent(token, owner, repoName, sourceFile, baseBranch)
	if err != nil {
		return testFiles
	}

	functions, classes := s.extractSymbols(sourceContent, sourceFile)
	if len(functions) == 0 && len(classes) == 0 {
		return testFiles
	}

	// Get all potential test files in the vicinity
	allTestPatterns := s.getTestFilePaths(sourceFile)

	// Add broader test file discovery
	sourceDir := filepath.Dir(sourceFile)
	packageName := filepath.Base(sourceDir)
	additionalPatterns := s.generatePackageTestPatterns(sourceFile, packageName)
	allTestPatterns = append(allTestPatterns, additionalPatterns...)

	for _, testPath := range allTestPatterns {
		content, err := s.githubClient.GetFileContent(token, owner, repoName, testPath, baseBranch)
		if err != nil {
			continue
		}

		// Check if the test file references any of the functions or classes
		if s.semanticAnalysis(content, functions, classes) {
			testFiles = append(testFiles, testPath)
		}
	}

	return testFiles
}

// extractSymbols extracts function and class names from source code
func (s *Service) extractSymbols(sourceContent, sourceFile string) ([]string, []string) {
	if strings.HasSuffix(sourceFile, ".py") {
		return s.extractPythonSymbols(sourceContent)
	}

	if strings.HasSuffix(sourceFile, ".js") || strings.HasSuffix(sourceFile, ".ts") ||
		strings.HasSuffix(sourceFile, ".jsx") || strings.HasSuffix(sourceFile, ".tsx") {
		return s.extractJSSymbols(sourceContent, sourceFile)
	}

	return []string{}, []string{}
}

// extractPythonSymbols extracts Python functions and classes
func (s *Service) extractPythonSymbols(sourceContent string) ([]string, []string) {
	var functions, classes []string

	funcPattern := regexp.MustCompile(`def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	classPattern := regexp.MustCompile(`class\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*[\(:]`)

	funcMatches := funcPattern.FindAllStringSubmatch(sourceContent, -1)
	for _, match := range funcMatches {
		if len(match) > 1 && !strings.HasPrefix(match[1], "_") {
			functions = append(functions, match[1])
		}
	}

	classMatches := classPattern.FindAllStringSubmatch(sourceContent, -1)
	for _, match := range classMatches {
		if len(match) > 1 {
			classes = append(classes, match[1])
		}
	}

	return functions, classes
}

// extractJSSymbols extracts JavaScript/TypeScript functions and classes
func (s *Service) extractJSSymbols(sourceContent, sourceFile string) ([]string, []string) {
	var functions, classes []string

	// Function patterns
	funcPatterns := []*regexp.Regexp{
		regexp.MustCompile(`function\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`),
		regexp.MustCompile(`(?:const|let|var)\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*(?:\([^)]*\)|[a-zA-Z_][a-zA-Z0-9_]*)\s*=>`),
		regexp.MustCompile(`export\s+(?:async\s+)?function\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`),
		regexp.MustCompile(`export\s+const\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*(?:\([^)]*\)|[a-zA-Z_][a-zA-Z0-9_]*)\s*=>`),
	}

	// Class patterns
	classPatterns := []*regexp.Regexp{
		regexp.MustCompile(`class\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:extends\s+[a-zA-Z_][a-zA-Z0-9_]*\s*)?\{`),
		regexp.MustCompile(`export\s+(?:default\s+)?class\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:extends\s+[a-zA-Z_][a-zA-Z0-9_]*\s*)?\{`),
	}

	// React components (JSX/TSX)
	if strings.HasSuffix(sourceFile, ".jsx") || strings.HasSuffix(sourceFile, ".tsx") {
		reactPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(?:const|let|var)\s+([A-Z][a-zA-Z0-9_]*)\s*=\s*(?:\([^)]*\)|[a-zA-Z_][a-zA-Z0-9_]*)\s*=>\s*(?:\(|\{|<)`),
			regexp.MustCompile(`export\s+(?:default\s+)?(?:const|let|var)\s+([A-Z][a-zA-Z0-9_]*)\s*=\s*(?:\([^)]*\)|[a-zA-Z_][a-zA-Z0-9_]*)\s*=>\s*(?:\(|\{|<)`),
			regexp.MustCompile(`export\s+default\s+(?:function\s+)?([A-Z][a-zA-Z0-9_]*)`),
		}
		funcPatterns = append(funcPatterns, reactPatterns...)
	}

	// TypeScript types (TS/TSX)
	if strings.HasSuffix(sourceFile, ".ts") || strings.HasSuffix(sourceFile, ".tsx") {
		typePatterns := []*regexp.Regexp{
			regexp.MustCompile(`(?:export\s+)?interface\s+([A-Z][a-zA-Z0-9_]*)\s*(?:extends\s+[^{]+)?\s*\{`),
			regexp.MustCompile(`(?:export\s+)?type\s+([A-Z][a-zA-Z0-9_]*)\s*=`),
			regexp.MustCompile(`(?:export\s+)?(?:const\s+)?enum\s+([A-Z][a-zA-Z0-9_]*)\s*\{`),
		}
		classPatterns = append(classPatterns, typePatterns...)
	}

	// Extract functions
	for _, pattern := range funcPatterns {
		matches := pattern.FindAllStringSubmatch(sourceContent, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" && !strings.HasPrefix(match[1], "_") {
				functions = append(functions, match[1])
			}
		}
	}

	// Extract classes
	for _, pattern := range classPatterns {
		matches := pattern.FindAllStringSubmatch(sourceContent, -1)
		for _, match := range matches {
			if len(match) > 1 && match[1] != "" {
				classes = append(classes, match[1])
			}
		}
	}

	return functions, classes
}

// semanticAnalysis checks if test content references any of the given functions or classes
func (s *Service) semanticAnalysis(testContent string, functions, classes []string) bool {
	// Check function references
	for _, function := range functions {
		if s.checkSymbolReferences(testContent, function, false) {
			return true
		}
	}

	// Check class references
	for _, class := range classes {
		if s.checkSymbolReferences(testContent, class, true) {
			return true
		}
	}

	return false
}

// checkSymbolReferences checks if a symbol (function or class) is referenced in test content
func (s *Service) checkSymbolReferences(testContent, symbol string, isClass bool) bool {
	patterns := s.buildSymbolPatterns(symbol, isClass)

	return matchAnyPattern(patterns, testContent)
}

// buildSymbolPatterns creates patterns to find symbol references
func (s *Service) buildSymbolPatterns(symbol string, isClass bool) []string {
	patterns := []string{
		// Common patterns for both functions and classes
		regexp.QuoteMeta(symbol) + `\s*\(`,                                // Direct calls/instantiation
		`(test|it|describe)\s*\(\s*['"][^'"]*` + regexp.QuoteMeta(symbol), // Test descriptions
		`(mock|patch|jest\.mock|vi\.mock).*` + regexp.QuoteMeta(symbol),   // Mocking
		`['"]` + regexp.QuoteMeta(symbol) + `['"]`,                        // String references
		`//.*` + regexp.QuoteMeta(symbol),                                 // Comments
		`@\w+.*` + regexp.QuoteMeta(symbol),                               // JSDoc/decorators
	}

	if isClass {
		// Class-specific patterns
		patterns = append(patterns, []string{
			`new\s+` + regexp.QuoteMeta(symbol) + `\s*\(`,         // new Class()
			`class\s+\w+\s+extends\s+` + regexp.QuoteMeta(symbol), // inheritance
			`implements\s+[^{]*` + regexp.QuoteMeta(symbol),       // interface implementation
			`:\s*` + regexp.QuoteMeta(symbol) + `[\s<>;,]`,        // type annotations
			`<[^>]*` + regexp.QuoteMeta(symbol) + `[^>]*>`,        // generic types
			`\s+as\s+` + regexp.QuoteMeta(symbol),                 // type assertions
		}...)
	} else {
		// Function-specific patterns
		patterns = append(patterns, []string{
			`def\s+test.*` + regexp.QuoteMeta(symbol),              // Python test functions
			`render\s*\(\s*<\s*` + regexp.QuoteMeta(symbol),        // React testing
			`</?` + regexp.QuoteMeta(symbol) + `[\s/>]`,            // JSX usage
			`\w+\s*=\s*\{\s*` + regexp.QuoteMeta(symbol) + `\s*\}`, // JSX props
			`expect\s*\(\s*[^)]*` + regexp.QuoteMeta(symbol),       // test assertions
		}...)
	}

	return patterns
}

// removeDuplicateStrings removes duplicate strings from a slice
func (s *Service) removeDuplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, str := range slice {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}

// matchAnyPattern compiles and matches patterns against content, returning true if any match
func matchAnyPattern(patterns []string, content string) bool {
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(content) {
			return true
		}
	}
	return false
}
